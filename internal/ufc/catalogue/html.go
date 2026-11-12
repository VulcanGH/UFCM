// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/autobrr/autobrr/pkg/errors"

	"golang.org/x/net/html"
)

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func walk(n *html.Node, fn func(*html.Node) bool) {
	if !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	walk(n, func(c *html.Node) bool {
		if c.Type == html.ElementNode && strings.Contains(attr(c, "class"), "mw-editsection") {
			return false
		}
		if c.Type == html.ElementNode && (c.Data == "sup" || c.Data == "script" || c.Data == "style" || c.Data == "table") {
			return false
		}
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
		return true
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

type listRow struct {
	Facts
	Link    string
	Blocked bool
}

// parseList deliberately requires both named sections; partial discovery is not success.
func parseList(content string, revision int64) ([]listRow, error) {
	if err := validateTables(content); err != nil {
		return nil, err
	}
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil, errors.Wrap(err, "parse event list")
	}
	var rows []listRow
	section := ""
	seen := map[string]int{}
	var parseErr error
	walk(doc, func(n *html.Node) bool {
		if parseErr != nil {
			return false
		}
		if n.Type != html.ElementNode {
			return true
		}
		if n.Data == "h2" {
			section = ""
			name := Normalize(nodeText(n))
			if name == "scheduled events" || name == "past events" {
				section = name
			}
		}
		if n.Data != "table" {
			return true
		}
		if section == "" {
			return false
		}
		grid, e := tableGrid(n)
		if e != nil {
			parseErr = e
			return false
		}
		if len(grid) < 1 {
			return false
		}
		columns := map[string]int{}
		for i, c := range grid[0] {
			if c != nil {
				columns[Normalize(nodeText(c))] = i
			}
		}
		ec, eventOK := columns["event"]
		dc, dateOK := columns["date"]
		if !eventOK || !dateOK {
			return false
		}
		if section == "past events" {
			if _, ok := columns["#"]; !ok {
				parseErr = errors.New("past event sequence header missing")
				return false
			}
		}
		seen[section]++
		for _, cells := range grid[1:] {
			if ec >= len(cells) || dc >= len(cells) || cells[ec] == nil || cells[dc] == nil {
				parseErr = errors.New("event row has missing required cells")
				break
			}
			name := nodeText(cells[ec])
			if name == "" {
				parseErr = errors.New("event row has empty name")
				break
			}
			f := Facts{Name: name, ListRevision: revision, Section: section, Status: "unknown", Type: "fight_night"}
			if m := numberPattern.FindStringSubmatch(name); len(m) > 0 && m[1] == "" && strings.HasPrefix(name, m[0]) {
				f.Type = "numbered"
			}
			if section == "scheduled events" {
				f.Status = "scheduled"
			}
			dateText := nodeText(cells[dc])
			f.Date = calendarDate(dateText)
			if f.Date == nil && !unknownDate(dateText) {
				addReview(&f, "unrecognised_date")
			}
			row := listRow{Facts: f}
			var links []string
			walk(cells[ec], func(c *html.Node) bool {
				if c.Type == html.ElementNode && c.Data == "sup" {
					return false
				}
				if c.Data == "a" {
					href := attr(c, "href")
					if strings.HasPrefix(href, "/wiki/") {
						links = append(links, strings.TrimPrefix(href, "/wiki/"))
					}
				}
				return true
			})
			if len(links) == 1 {
				row.Link, err = url.PathUnescape(links[0])
				if err != nil {
					parseErr = errors.Wrap(err, "invalid event link")
					break
				}
				row.Title = strings.ReplaceAll(row.Link, "_", " ")
				if strings.Contains(row.Title, "#") {
					row.Blocked = true
					addReview(&row.Facts, "section_link")
				}
			} else {
				addReview(&row.Facts, "missing_or_competing_links")
				row.Blocked = true
			}
			if section == "past events" {
				if i := columns["#"]; i < len(cells) && cells[i] != nil {
					row.OverallSequence = nodeText(cells[i])
				}
			}
			marker := Normalize(name + " " + nodeText(cells[dc]))
			for _, cell := range cells {
				if cell != nil {
					value := Normalize(nodeText(cell))
					if value == "canceled" || value == "cancelled" || value == "postponed" {
						marker += " " + value
					}
				}
			}
			if strings.Contains(marker, "cancelled") || strings.Contains(marker, "canceled") {
				row.Status = "cancelled"
				addReview(&row.Facts, "cancelled_event")
			}
			if strings.Contains(marker, "postponed") {
				row.Status = "postponed"
				addReview(&row.Facts, "postponed_event")
			}
			addName(&row.Facts, name, !row.Blocked && row.Status != "cancelled" && row.Status != "postponed")
			rows = append(rows, row)
			if len(rows) > 2500 {
				parseErr = errors.New("event row limit exceeded")
				break
			}
		}
		return false
	})
	if parseErr != nil {
		return nil, parseErr
	}
	if seen["scheduled events"] != 1 || seen["past events"] != 1 || len(rows) == 0 {
		return nil, errors.New("incomplete event discovery: expected two semantic event tables")
	}
	return rows, nil
}

func tableGrid(table *html.Node) ([][]*html.Node, error) {
	type span struct {
		node *html.Node
		left int
	}
	active := map[int]span{}
	remaining := map[*html.Node]int{}
	walk(table, func(n *html.Node) bool {
		if n != table && n.Data == "table" {
			return false
		}
		if n.Data == "tr" {
			remaining[n.Parent]++
			return false
		}
		return true
	})
	var group *html.Node
	var grid [][]*html.Node
	var gridErr error
	walk(table, func(n *html.Node) bool {
		if n != table && n.Data == "table" {
			return false
		}
		if n.Data != "tr" {
			return true
		}
		if group != n.Parent {
			active = map[int]span{}
			group = n.Parent
		}
		remaining[n.Parent]--
		cells := make([]*html.Node, 64)
		for i, s := range active {
			cells[i] = s.node
			s.left--
			if s.left == 0 {
				delete(active, i)
			} else {
				active[i] = s
			}
		}
		col, last := 0, 0
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Data != "td" && c.Data != "th" {
				continue
			}
			for col < 64 && cells[col] != nil {
				col++
			}
			cs, rs := 1, 1
			for key, dest := range map[string]*int{"colspan": &cs, "rowspan": &rs} {
				if v := attr(c, key); v != "" {
					match := htmlInteger.FindStringSubmatch(v)
					if len(match) < 2 {
						gridErr = errors.New("invalid table span")
						return false
					}
					num, e := strconv.Atoi(match[1])
					if e != nil || num > 2500 {
						gridErr = errors.New("invalid table span")
						return false
					}
					if num == 0 {
						if key == "rowspan" {
							num = remaining[n.Parent] + 1
						} else {
							num = 1
						}
					}
					*dest = num
				}
			}
			if col+cs > 64 {
				gridErr = errors.New("event table column limit exceeded")
				return false
			}
			for j := 0; j < cs; j++ {
				if cells[col+j] != nil {
					gridErr = errors.New("overlapping table spans")
					return false
				}
				cells[col+j] = c
				if rs > 1 {
					active[col+j] = span{c, rs - 1}
				}
			}
			col += cs
		}
		for i, c := range cells {
			if c != nil {
				last = i + 1
			}
		}
		grid = append(grid, cells[:last])
		if len(grid) > 2501 {
			gridErr = errors.New("event table row limit exceeded")
		}
		return false
	})
	return grid, gridErr
}

// HTML integer attributes accept a leading number followed by invalid trailing text.
var htmlInteger = regexp.MustCompile(`^\s*\+?([0-9]+)`)

func validateTables(content string) error {
	tokens := html.NewTokenizer(strings.NewReader(content))
	depth := 0
	for {
		kind := tokens.Next()
		switch kind {
		case html.StartTagToken:
			name, _ := tokens.TagName()
			if string(name) == "table" {
				depth++
			}
		case html.EndTagToken:
			name, _ := tokens.TagName()
			if string(name) == "table" {
				depth--
				if depth < 0 {
					return errors.New("malformed event HTML table")
				}
			}
		case html.ErrorToken:
			if tokens.Err() != io.EOF {
				return errors.Wrap(tokens.Err(), "read event HTML")
			}
			if depth != 0 {
				return errors.New("truncated event HTML table")
			}
			return nil
		}
	}
}

func calendarDate(value string) *string {
	for _, layout := range []string{"2006-01-02", "Jan 2, 2006", "January 2, 2006", "2 January 2006"} {
		if d, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			s := d.Format("2006-01-02")
			return &s
		}
	}
	return nil
}

func unknownDate(v string) bool {
	v = Normalize(v)
	return v == "" || v == "tba" || v == "tbd" || v == "to be announced" || v == "—" || v == "n/a"
}

var numberPattern = regexp.MustCompile(`^UFC\s+(?:(Fight Night|Vegas|on ESPN\+|on ESPN|on ABC|Freedom)\s+)?([0-9]+(?:\.[0-9]+)?)\b`)
var headlinerPattern = regexp.MustCompile(`:\s+(.+?)\s+vs\.?\s+(.+?)(?:\s+[0-9]+)?$`)
var articleDatePattern = regexp.MustCompile(`\b(?:January|February|March|April|May|June|July|August|September|October|November|December) [0-9]{1,2},? [0-9]{4}\b`)

func addName(f *Facts, name string, current bool) {
	addAlias(f, Alias{Namespace: "title", Value: name, Current: current, Verified: current})
	for _, m := range numberPattern.FindAllStringSubmatch(name, -1) {
		ns := map[string]string{"": "numbered", "Fight Night": "fight_night", "Vegas": "vegas", "on ESPN+": "espn_plus", "on ESPN": "espn", "on ABC": "abc", "Freedom": "freedom"}[m[1]]
		if ns == "numbered" && current {
			f.Type = "numbered"
		}
		addAlias(f, Alias{Namespace: "ufc." + ns, Value: m[2], Current: current, Verified: current})
	}
	if current && len(f.Headliners) == 0 {
		if m := headlinerPattern.FindStringSubmatch(name); len(m) > 0 {
			f.Headliners = []string{m[1], m[2]}
		}
	}
}

func enrich(f *Facts, content string) error {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return errors.Wrap(err, "parse event article")
	}
	var lead *html.Node
	stopped := false
	walk(doc, func(n *html.Node) bool {
		if stopped {
			return false
		}
		if n.Type != html.ElementNode {
			return true
		}
		if n.Data == "table" || n.Data == "aside" {
			return false
		}
		if n.Data == "h2" {
			stopped = true
			return false
		}
		if n.Data == "p" && strings.Contains(nodeText(n), "mixed martial arts") && lead == nil {
			lead = n
			return false
		}
		return true
	})
	if lead == nil {
		return errors.New("dedicated event lead unavailable")
	}
	text := nodeText(lead)
	if len(text) > 16384 {
		return errors.New("event lead exceeds enrichment limit")
	}
	cut := strings.Index(text, " is ")
	if cut < 0 {
		cut = strings.Index(text, " was ")
	}
	if cut < 0 {
		return errors.New("event lead identity unclear")
	}
	prefix := text[:cut]
	if strings.Contains(Normalize(text[:min(len(text), cut+100)]), "cancelled") || strings.Contains(Normalize(text[:min(len(text), cut+100)]), "canceled") {
		f.Status = "cancelled"
		addReview(f, "cancelled_event")
		for i := range f.Aliases {
			f.Aliases[i].Current = false
			f.Aliases[i].Verified = false
		}
		return nil
	}
	var names []string
	walk(lead, func(n *html.Node) bool {
		if n.Data == "b" {
			v := nodeText(n)
			if strings.Contains(prefix, v) {
				names = append(names, v)
			}
			return false
		}
		return true
	})
	for _, name := range names {
		current := true
		if idx := strings.Index(prefix, name); idx >= 0 {
			before := Normalize(prefix[:idx])
			if strings.HasSuffix(before, "originally known as") || strings.HasSuffix(before, "originally billed as") || strings.HasSuffix(before, "formerly") {
				current = false
			}
		}
		beforeType := f.Type
		if current && len(f.Headliners) == 2 {
			if m := headlinerPattern.FindStringSubmatch(name); len(m) > 0 && (Normalize(m[1]) != Normalize(f.Headliners[0]) || Normalize(m[2]) != Normalize(f.Headliners[1])) {
				addReview(f, "article_list_headliner_conflict")
			}
		}
		addName(f, name, current)
		if current && beforeType != f.Type {
			addReview(f, "article_list_type_conflict")
			f.Type = beforeType
		}
	}
	firstSentence := text[cut:]
	if end := strings.Index(firstSentence, ". "); end >= 0 {
		firstSentence = firstSentence[:end]
	}
	if m := articleDatePattern.FindString(firstSentence); m != "" {
		m = strings.ReplaceAll(m, ",", "")
		d, e := time.Parse("January 2 2006", m)
		if e == nil && f.Date != nil && d.Format("2006-01-02") != *f.Date {
			addReview(f, "article_list_date_conflict")
		}
	}
	if (strings.Contains(firstSentence, "took place on") || strings.Contains(firstSentence, "held on")) && !strings.Contains(firstSentence, "scheduled") && !strings.Contains(firstSentence, "planned") {
		if len(f.Review) == 0 {
			f.Status = "completed"
		}
	}
	claims := map[string]string{}
	for _, a := range f.Aliases {
		if !a.Current || !strings.HasPrefix(a.Namespace, "ufc.") {
			continue
		}
		if prev, ok := claims[a.Namespace]; ok && prev != a.Value {
			addReview(f, "competing_"+a.Namespace)
		}
		claims[a.Namespace] = a.Value
	}
	if len(f.Review) > 0 {
		for i := range f.Aliases {
			if strings.HasPrefix(f.Aliases[i].Namespace, "ufc.") {
				f.Aliases[i].Verified = false
			}
		}
	}
	background := false
	walk(doc, func(n *html.Node) bool {
		if n.Data == "table" {
			return false
		}
		if n.Data == "h2" {
			background = Normalize(nodeText(n)) == "background"
		}
		if background && n.Data == "p" {
			text := nodeText(n)
			if formerBilling.MatchString(text) {
				if alias := numberPattern.FindString(formerBilling.ReplaceAllString(text, "UFC ")); alias != "" {
					addName(f, alias, false)
				}
			}
			return false
		}
		return true
	})
	return nil
}

var formerBilling = regexp.MustCompile(`^The event was (?:originally|initially) (?:billed|known|announced) as UFC `)

func identityTitle(title string) bool {
	if strings.Contains(title, "#") || strings.Contains(title, "events") || strings.Contains(title, " in UFC") {
		return false
	}
	return dedicatedTitle.MatchString(title)
}

var dedicatedTitle = regexp.MustCompile(`^UFC (?:[0-9]+(?:\.5)?(?:$|:)|Freedom(?: [0-9]+$|:)|Fight Night(?: [0-9]+$|:)|on (?:ESPN\+?|ABC|Fox|FX|Fuel TV|Versus)(?: [0-9]+$|:))`)

func pageKey(id int64) string { return fmt.Sprintf("wikipedia.en.page_id:%d", id) }
