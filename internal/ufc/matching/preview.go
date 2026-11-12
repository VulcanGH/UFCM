// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package matching explains release titles without making acquisition decisions.
package matching

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"golang.org/x/text/unicode/norm"
)

// Identifier preserves a release's numbering namespace.
type Identifier struct {
	Namespace string `json:"namespace"`
	Value     string `json:"value"`
}

// Candidate explains supporting evidence and conflicts for one local event.
type Candidate struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Date      *string  `json:"date"`
	Reasons   []string `json:"reasons"`
	Conflicts []string `json:"conflicts"`
}

// Classification reports independent component or quality observations.
type Classification struct {
	Status string   `json:"status"`
	Values []string `json:"values"`
}

// Preview is a versioned, side-effect-free matching explanation.
type Preview struct {
	Version     string         `json:"version"`
	Title       string         `json:"title"`
	Normalized  string         `json:"normalized"`
	Status      string         `json:"status"`
	Identifiers []Identifier   `json:"identifiers"`
	Candidates  []Candidate    `json:"candidates"`
	Components  Classification `json:"components"`
	Quality     Classification `json:"quality"`
	MainCard    MainCardCheck  `json:"main_card"`
	Notes       []string       `json:"notes"`
}

var numbered = regexp.MustCompile(`\bufc[ ._-]+([0-9]+(?:\.5)?)\b`)
var fightNight = regexp.MustCompile(`\bufc[ ._-]+(?:fight[ ._-]*night|fn)[ ._-]+([0-9]+)\b`)
var vegas = regexp.MustCompile(`\bufc[ ._-]+vegas[ ._-]+([0-9]+)\b`)
var espn = regexp.MustCompile(`\bufc[ ._-]+(?:on[ ._-]+)?espn([ ._-]*(?:\+|plus))?[ ._-]*([0-9]+)\b`)
var abc = regexp.MustCompile(`\bufc[ ._-]+(?:on[ ._-]+)?abc[ ._-]+([0-9]+)\b`)
var freedom = regexp.MustCompile(`\bufc[ ._-]+(?:on[ ._-]+)?freedom[ ._-]+([0-9]+)\b`)
var datePattern = regexp.MustCompile(`\b((?:19|20)[0-9]{2})[ ._-]([0-9]{2})[ ._-]([0-9]{2})\b`)

func folded(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		if r == '_' {
			return ' '
		}
		return unicode.ToLower(r)
	}, norm.NFD.String(norm.NFKC.String(value)))
}

func words(value string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, folded(value))), " ")
}

func contains(title, phrase string) bool {
	return phrase != "" && strings.Contains(" "+title+" ", " "+phrase+" ")
}

func identifiers(title string) []Identifier {
	result := []Identifier{}
	seen := map[Identifier]bool{}
	add := func(namespace, value string) {
		id := Identifier{namespace, value}
		if !seen[id] {
			result = append(result, id)
			seen[id] = true
		}
	}
	for _, pattern := range []struct {
		re        *regexp.Regexp
		namespace string
	}{{numbered, "ufc.numbered"}, {fightNight, "ufc.fight_night"}, {vegas, "ufc.vegas"}, {abc, "ufc.abc"}, {freedom, "ufc.freedom"}} {
		for _, m := range pattern.re.FindAllStringSubmatchIndex(title, -1) {
			if pattern.namespace == "ufc.numbered" {
				prefix := strings.Fields(words(title[:m[0]]))
				if len(prefix) > 0 && prefix[len(prefix)-1] == "noche" {
					continue
				}
			}
			add(pattern.namespace, title[m[2]:m[3]])
		}
	}
	for _, m := range espn.FindAllStringSubmatch(title, -1) {
		namespace := "ufc.espn"
		if m[1] != "" {
			namespace = "ufc.espn_plus"
		}
		add(namespace, m[2])
	}
	return result
}

// ClassifyComponents recognizes explicit coverage without guessing a missing card.
func ClassifyComponents(title string) Classification {
	title = words(title)
	result := Classification{Status: "unknown", Values: []string{}}
	early := false
	for _, phrase := range []string{"early preliminaries", "early preliminary", "early prelims", "early prelim", "earlyprelims", "earlyprelim"} {
		if contains(words(title), phrase) {
			early = true
			title = strings.ReplaceAll(title, phrase, " ")
		}
	}
	if early {
		result.Values = append(result.Values, "early_prelims")
	}
	for _, item := range []struct {
		key     string
		phrases []string
	}{
		{"main_card", []string{"main card"}}, {"prelims", []string{"prelims", "prelim", "preliminaries", "preliminary", "preliminary card", "preliminary fights"}},
		{"countdown", []string{"countdown"}}, {"embedded", []string{"embedded"}}, {"weigh_ins", []string{"weigh ins", "weigh in", "weighins"}},
	} {
		for _, phrase := range item.phrases {
			if contains(words(title), phrase) {
				result.Values = append(result.Values, item.key)
				break
			}
		}
	}
	if len(result.Values) > 0 {
		result.Status = "explicit"
	}
	return result
}

// AssessQuality reports title assertions, never verified media characteristics.
func AssessQuality(title string) Classification {
	title = words(title)
	result := Classification{Status: "unknown", Values: []string{}}
	resolutions := 0
	for _, item := range []struct {
		key     string
		phrases []string
	}{
		{"2160p", []string{"2160p", "4k"}}, {"1080p", []string{"1080p"}}, {"1080i", []string{"1080i"}}, {"720p", []string{"720p"}}, {"480p", []string{"480p"}},
		{"WEB-DL", []string{"web dl", "webdl"}}, {"WEBRip", []string{"webrip", "web rip"}}, {"HDTV", []string{"hdtv"}},
		{"H.264", []string{"h264", "h 264", "x264"}}, {"H.265", []string{"h265", "h 265", "x265", "hevc"}}, {"AV1", []string{"av1"}},
	} {
		for _, phrase := range item.phrases {
			if contains(title, phrase) {
				result.Values = append(result.Values, item.key)
				if item.key == "2160p" || item.key == "1080p" || item.key == "1080i" || item.key == "720p" || item.key == "480p" {
					resolutions++
				}
				break
			}
		}
	}
	if len(result.Values) > 0 {
		result.Status = "title assertions"
	}
	if resolutions > 1 {
		result.Status = "conflicting resolutions"
	}
	return result
}

// Match compares a title with effective local catalogue facts and explains uncertainty.
func Match(title string, events []catalogue.Event) Preview {
	return MatchWithTerms(title, events, nil)
}

// MatchWithTerms uses the same policy for preview, manual search and acquisition.
func MatchWithTerms(title string, events []catalogue.Event, terms []string) Preview {
	return match(title, events, false, terms)
}

// MatchLibrary identifies local media independently of acquisition status eligibility.
func MatchLibrary(title string, events []catalogue.Event) Preview {
	return libraryIdentity(title, events)
}

func match(title string, events []catalogue.Event, library bool, terms []string) Preview {
	normalized := words(title)
	p := Preview{Version: "preview-4", Title: title, Normalized: normalized, Status: "unmatched", Identifiers: identifiers(folded(title)), Candidates: []Candidate{}, Components: ClassifyComponents(title), Quality: AssessQuality(title), Notes: []string{"Preview only. No acquisition decision or download is made.", "Quality values are unverified assertions from the title."}}
	dates := []string{}
	for _, m := range datePattern.FindAllStringSubmatch(folded(title), -1) {
		d := m[1] + "-" + m[2] + "-" + m[3]
		if _, err := time.Parse("2006-01-02", d); err == nil {
			dates = append(dates, d)
		}
	}
	p.MainCard = CheckMainCardWithTerms(title, p.Components, terms)
	covered := map[Identifier]bool{}
	for _, event := range events {
		if event.Corrections.Deleted {
			continue
		}
		c := Candidate{ID: event.ID, Name: event.Values.Name, Date: event.Values.Date, Reasons: []string{}, Conflicts: []string{}}
		strong := false
		matchedIDs := map[Identifier]bool{}
		for _, id := range p.Identifiers {
			for _, alias := range event.Aliases {
				if alias.Namespace == id.Namespace && alias.Value == id.Value {
					matchedIDs[id] = true
					covered[id] = true
					c.Reasons = append(c.Reasons, "Identifier: "+id.Namespace+" "+id.Value)
					if alias.Verified && alias.Current {
						strong = true
					} else {
						c.Conflicts = append(c.Conflicts, "Numbering alias is former or unverified")
					}
					break
				}
			}
		}
		name := words(event.Values.Name)
		if len(strings.Fields(name)) >= 3 && name != "ufc fight night" && name != "ufc on espn" && contains(normalized, name) {
			c.Reasons = append(c.Reasons, "Effective event name appears in title")
			strong = true
		}
		for _, alias := range event.Aliases {
			if alias.Namespace == "title" && len(strings.Fields(words(alias.Value))) >= 3 && contains(normalized, words(alias.Value)) {
				c.Reasons = append(c.Reasons, "Title alias: "+alias.Value)
				if alias.Current && alias.Verified {
					strong = true
				} else {
					c.Conflicts = append(c.Conflicts, "Title alias is former or unverified")
				}
			}
		}
		if len(event.Values.Headliners) == 2 {
			first, second := strings.Fields(words(event.Values.Headliners[0])), strings.Fields(words(event.Values.Headliners[1]))
			if len(first) > 0 && len(second) > 0 && first[len(first)-1] != second[len(second)-1] && contains(normalized, first[len(first)-1]) && contains(normalized, second[len(second)-1]) {
				c.Reasons = append(c.Reasons, "Both headliner surnames appear in title")
				for _, date := range dates {
					if event.Values.Date != nil && date == *event.Values.Date {
						strong = true
					}
				}
			}
		}
		if len(c.Reasons) == 0 {
			continue
		}
		for _, id := range p.Identifiers {
			if !matchedIDs[id] {
				c.Conflicts = append(c.Conflicts, "Uncorroborated identifier: "+id.Namespace+" "+id.Value)
			}
		}
		for _, date := range dates {
			if event.Values.Date == nil || date != *event.Values.Date {
				c.Conflicts = append(c.Conflicts, "Title date differs from or is missing in catalogue")
			} else {
				c.Reasons = append(c.Reasons, "Event date agrees")
			}
		}
		if !strong {
			c.Conflicts = append(c.Conflicts, "Insufficient corroboration for a confident identity")
		}
		if len(event.Review) > 0 {
			c.Conflicts = append(c.Conflicts, "Catalogue entry needs review")
		}
		if event.Values.Status == "cancelled" || event.Values.Status == "postponed" || (!library && event.Values.Status == "unknown") {
			c.Conflicts = append(c.Conflicts, "Event status: "+event.Values.Status)
		}
		p.Candidates = append(p.Candidates, c)
	}
	sort.Slice(p.Candidates, func(i, j int) bool {
		if len(p.Candidates[i].Conflicts) != len(p.Candidates[j].Conflicts) {
			return len(p.Candidates[i].Conflicts) < len(p.Candidates[j].Conflicts)
		}
		return p.Candidates[i].ID < p.Candidates[j].ID
	})
	for _, id := range p.Identifiers {
		if !covered[id] {
			p.Notes = append(p.Notes, "Identifier not found in local aliases: "+id.Namespace+" "+id.Value)
		}
	}
	if len(p.Candidates) > 1 {
		p.Status = "ambiguous"
	} else if len(p.Candidates) == 1 {
		p.Status = "matched"
		if len(p.Candidates[0].Conflicts) > 0 {
			p.Status = "needs review"
		}
	}
	if p.MainCard.Status == "inferred label" {
		if p.Status == "matched" {
			p.Components = Classification{Status: "inferred", Values: []string{"main_card"}}
			p.MainCard.Reason = "Main Card inferred: confident event match with no other component or excluded programming terms."
			p.Notes = append(p.Notes, "Main Card is inferred from the release naming convention, not an explicit label or verified file contents.")
		} else {
			p.MainCard.Status = "needs review"
			p.MainCard.Reason = "A confident event match is required before inferring Main Card from an unlabelled release."
		}
	}
	return p
}
