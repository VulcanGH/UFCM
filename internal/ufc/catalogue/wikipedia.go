// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/autobrr/autobrr/pkg/errors"
)

const wikipediaEndpoint = "https://en.wikipedia.org/w/api.php"
const wikipediaAgent = "UFCManager/0.1 (https://github.com/VulcanGH/UFCM; catalogue read API)"
const responseLimit = 4 << 20

type apiPage struct {
	PageID    int64  `json:"pageid"`
	NS        int    `json:"ns"`
	Title     string `json:"title"`
	Missing   bool   `json:"missing"`
	Revisions []struct {
		RevID int64 `json:"revid"`
	} `json:"revisions"`
}

type apiRedirect struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Fragment string `json:"tofragment"`
}
type apiResponse struct {
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
	Continue json.RawMessage `json:"continue"`
	Parse    *struct {
		Text   string `json:"text"`
		Title  string `json:"title"`
		PageID int64  `json:"pageid"`
		RevID  int64  `json:"revid"`
	} `json:"parse"`
	Query *struct {
		Pages      []apiPage     `json:"pages"`
		Redirects  []apiRedirect `json:"redirects"`
		Normalized []apiRedirect `json:"normalized"`
	} `json:"query"`
}

// Wikipedia reads the fixed public MediaWiki endpoint without credentials.
type Wikipedia struct {
	client           *http.Client
	enrichmentLimit  int
	listRevision     int64
	articleRevisions map[int64]int64
	backoff          time.Duration
}

// NewWikipedia returns a bounded native reader; no configurable remote URL is accepted.
func NewWikipedia() *Wikipedia {
	return &Wikipedia{client: &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, enrichmentLimit: 32, backoff: time.Second}
}

func (w *Wikipedia) request(ctx context.Context, values url.Values) (*apiResponse, error) {
	values.Set("format", "json")
	values.Set("formatversion", "2")
	values.Set("maxlag", "5")
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, wikipediaEndpoint+"?"+values.Encode(), nil)
		if err != nil {
			return nil, errors.Wrap(err, "build Wikipedia request")
		}
		req.Header.Set("User-Agent", wikipediaAgent)
		res, err := w.client.Do(req)
		if err != nil {
			return nil, errors.Wrap(err, "Wikipedia request failed")
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, responseLimit+1))
		closeErr := res.Body.Close()
		if readErr != nil {
			return nil, errors.Wrap(readErr, "read Wikipedia response")
		}
		if closeErr != nil {
			return nil, errors.Wrap(closeErr, "close Wikipedia response")
		}
		if len(body) > responseLimit {
			return nil, errors.New("Wikipedia response size limit exceeded")
		}
		var response apiResponse
		decodeErr := json.Unmarshal(body, &response)
		retry := res.StatusCode == 429 || res.StatusCode >= 500 || (response.Error != nil && response.Error.Code == "maxlag")
		if retry && attempt < 2 {
			delay := w.backoff * time.Duration(1<<attempt)
			if seconds, e := strconv.Atoi(res.Header.Get("Retry-After")); e == nil && seconds > 0 {
				delay = max(delay, time.Duration(seconds)*time.Second)
			}
			if at, e := http.ParseTime(res.Header.Get("Retry-After")); e == nil {
				delay = max(delay, time.Until(at))
			}
			if delay > 30*time.Second {
				return nil, errors.New("Wikipedia retry delay exceeds refresh budget")
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if res.StatusCode != http.StatusOK {
			return nil, errors.New("Wikipedia HTTP status %d", res.StatusCode)
		}
		if decodeErr != nil {
			return nil, errors.Wrap(decodeErr, "invalid Wikipedia JSON")
		}
		if response.Error != nil {
			return nil, errors.New("Wikipedia API rejected request (%s)", response.Error.Code)
		}
		if len(response.Continue) > 0 {
			return nil, errors.New("incomplete Wikipedia response")
		}
		return &response, nil
	}
	return nil, errors.New("Wikipedia retry budget exhausted")
}

func (w *Wikipedia) parse(ctx context.Context, revision int64, title string) (*apiResponse, error) {
	values := url.Values{"action": {"parse"}, "prop": {"text|revid"}}
	if revision > 0 {
		values.Set("oldid", strconv.FormatInt(revision, 10))
	} else {
		values.Set("page", title)
	}
	res, err := w.request(ctx, values)
	if err != nil {
		return nil, err
	}
	if res.Parse == nil || res.Parse.RevID <= 0 || res.Parse.PageID <= 0 || res.Parse.Text == "" {
		return nil, errors.New("incomplete Wikipedia parse response")
	}
	if revision > 0 && res.Parse.RevID != revision {
		return nil, errors.New("Wikipedia revision mismatch")
	}
	return res, nil
}

func (w *Wikipedia) resolve(ctx context.Context, titles []string) (map[string]apiPage, map[string]string, error) {
	result := map[string]apiPage{}
	problems := map[string]string{}
	for start := 0; start < len(titles); start += 50 {
		batch := titles[start:min(start+50, len(titles))]
		res, err := w.request(ctx, url.Values{"action": {"query"}, "titles": {strings.Join(batch, "|")}, "redirects": {"1"}, "prop": {"info|revisions"}, "rvprop": {"ids"}})
		if err != nil {
			return nil, nil, err
		}
		if res.Query == nil {
			return nil, nil, errors.New("missing Wikipedia query metadata")
		}
		pages := map[string]apiPage{}
		ambiguous := map[string]bool{}
		redirects := map[string]apiRedirect{}
		for _, p := range res.Query.Pages {
			if prior, ok := pages[p.Title]; ok && prior.PageID != p.PageID {
				ambiguous[p.Title] = true
			}
			pages[p.Title] = p
		}
		for _, r := range append(res.Query.Normalized, res.Query.Redirects...) {
			redirects[r.From] = r
		}
		for _, title := range batch {
			resolved := title
			visited := map[string]bool{}
			for {
				r, ok := redirects[resolved]
				if !ok {
					break
				}
				if r.Fragment != "" || visited[resolved] || len(visited) > 8 {
					problems[title] = "section_or_cyclic_redirect"
					break
				}
				visited[resolved] = true
				resolved = r.To
			}
			if problems[title] != "" {
				continue
			}
			page, ok := pages[resolved]
			if ambiguous[resolved] {
				problems[title] = "competing_page_metadata"
				continue
			}
			if !ok || page.Missing || page.PageID <= 0 || page.NS != 0 || len(page.Revisions) != 1 {
				problems[title] = "missing_page_metadata"
				continue
			}
			if !identityTitle(page.Title) {
				problems[title] = "not_dedicated_event_page"
				continue
			}
			result[title] = page
		}
	}
	return result, problems, nil
}

// Read refreshes scheduled events and their individual transitions into history.
func (w *Wikipedia) Read(ctx context.Context, known []Facts) (*Snapshot, error) {
	return w.ReadScope(ctx, known, "upcoming")
}

// ReadScope limits article resolution to the requested catalogue operation.
func (w *Wikipedia) ReadScope(ctx context.Context, known []Facts, scope string) (*Snapshot, error) {
	parsed, err := w.parse(ctx, w.listRevision, "List of UFC events")
	if err != nil {
		return nil, err
	}
	rows, err := parseList(parsed.Parse.Text, parsed.Parse.RevID)
	if err != nil {
		return nil, err
	}
	selected := scopedRows(rows, known, scope)
	snapshot, err := w.readRows(ctx, known, selected, parsed.Parse.RevID)
	if err != nil {
		return nil, err
	}
	snapshot.Discovered = len(rows)
	if scope == "upcoming" {
		transitionIDs := map[int64]bool{}
		for _, f := range known {
			if f.Section == "scheduled events" {
				transitionIDs[f.PageID] = true
			}
		}
		facts := []Facts{}
		for _, f := range snapshot.Facts {
			if f.Section != "past events" || transitionIDs[f.PageID] {
				facts = append(facts, f)
			}
		}
		snapshot.Facts = facts
	}
	for _, row := range rows {
		snapshot.ListRows = append(snapshot.ListRows, row.Facts)
	}
	for i := range snapshot.Facts {
		f := &snapshot.Facts[i]
		if f.Section == "past events" && f.Date != nil && *f.Date <= time.Now().UTC().Format("2006-01-02") && f.Status == "unknown" {
			f.Status = "completed"
		}
		f.forceArticle = !f.articleFetched && (scope == "rebuild" || f.ArticleRevision > 0 && f.ArticleRevision != f.articleTarget)
	}
	return snapshot, nil
}

func routineRows(rows []listRow) []listRow {
	upcoming := []listRow{}
	past := []listRow{}
	for _, row := range rows {
		if row.Section == "scheduled events" {
			upcoming = append(upcoming, row)
		} else if row.Date != nil && row.Status != "cancelled" && row.Status != "postponed" {
			past = append(past, row)
		}
	}
	sort.SliceStable(upcoming, func(i, j int) bool {
		if upcoming[i].Date == nil {
			return false
		}
		if upcoming[j].Date == nil {
			return true
		}
		return *upcoming[i].Date < *upcoming[j].Date
	})
	sort.SliceStable(past, func(i, j int) bool { return *past[i].Date > *past[j].Date })
	return append(upcoming, past[:min(RecentCompletedLimit, len(past))]...)
}

func (w *Wikipedia) readRows(ctx context.Context, known []Facts, rows []listRow, listRevision int64) (*Snapshot, error) {
	snapshot := &Snapshot{ListRevision: listRevision, Discovered: len(rows), Facts: []Facts{}, Diagnostics: []Diagnostic{}}
	var titles []string
	seenTitles := map[string]bool{}
	for _, r := range rows {
		if !r.Blocked && !seenTitles[r.Title] {
			titles = append(titles, r.Title)
			seenTitles[r.Title] = true
		}
	}
	pages, problems, err := w.resolve(ctx, titles)
	if err != nil {
		return nil, errors.Wrap(err, "incomplete discovery metadata")
	}
	previous := map[int64]Facts{}
	knownTitles := map[string]int64{}
	for _, f := range known {
		previous[f.PageID] = f
		knownTitles[f.Title] = f.PageID
		if f.LinkedTitle != "" {
			knownTitles[f.LinkedTitle] = f.PageID
		}
	}
	enriched := 0
	ids := map[int64]int{}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if row.Blocked || problems[row.Title] != "" {
			reason := problems[row.Title]
			if reason == "" {
				reason = strings.Join(row.Review, ",")
			}
			addDiagnostic(snapshot, Diagnostic{row.Name, row.Title, reason})
			continue
		}
		page := pages[row.Title]
		if owner, ok := knownTitles[row.Title]; ok && owner != page.PageID {
			addDiagnostic(snapshot, Diagnostic{row.Name, row.Title, "redirect_changes_known_identity"})
			continue
		}
		f := row.Facts
		f.PageID = page.PageID
		f.Reference = pageKey(page.PageID)
		f.LinkedTitle = f.Title
		f.Title = page.Title
		f.RetrievedAt = time.Now().UTC()
		if index, ok := ids[f.PageID]; ok {
			addReview(&snapshot.Facts[index], "contradictory_source_links")
			addReview(&f, "contradictory_source_links")
			addDiagnostic(snapshot, Diagnostic{f.Name, f.Title, "contradictory_source_links"})
			continue
		}
		old, hasOld := previous[f.PageID]
		if hasOld {
			f.ArticleRevision = old.ArticleRevision
		}
		revision := page.Revisions[0].RevID
		if pinned := w.articleRevisions[page.PageID]; pinned > 0 {
			revision = pinned
		}
		f.articleTarget = revision
		fresh := false
		if f.Status == "cancelled" || f.Status == "postponed" {
			fresh = true
		} else if (f.ArticleRevision != revision || hasOld && !sameBase(old, f)) && enriched < w.enrichmentLimit {
			enriched++
			article, e := w.parse(ctx, revision, "")
			if e == nil && (article.Parse.PageID != page.PageID || !identityTitle(article.Parse.Title)) {
				addReview(&f, "article_identity_conflict")
				e = errors.New("contradictory article identity")
			}
			if e == nil {
				e = enrich(&f, article.Parse.Text)
				if e == nil {
					f.ArticleRevision = revision
					f.articleFetched = true
					fresh = true
				}
			}
			if e != nil {
				addReview(&f, "article_enrichment_unavailable")
			}
		} else if f.ArticleRevision == 0 {
			addReview(&f, "article_enrichment_pending")
		}
		if !fresh && hasOld {
			if f.Status == "unknown" && old.Status != "unknown" {
				f.Status = old.Status
			}
			for _, a := range old.Aliases {
				if a.Namespace == "title" && a.Value != f.Name && old.Name != f.Name {
					a.Current = false
					a.Verified = false
				}
				addAlias(&f, a)
			}
			if len(f.Headliners) == 0 {
				f.Headliners = append([]string{}, old.Headliners...)
			}
			for _, reason := range old.Review {
				if reason != "article_enrichment_pending" && reason != "article_enrichment_unavailable" {
					addReview(&f, reason)
				}
			}
		}
		ids[f.PageID] = len(snapshot.Facts)
		snapshot.Facts = append(snapshot.Facts, f)
	}
	owners := map[string][]int{}
	for i, f := range snapshot.Facts {
		for _, a := range f.Aliases {
			if a.Current && a.Verified && strings.HasPrefix(a.Namespace, "ufc.") {
				key := a.Namespace + ":" + Normalize(a.Value)
				owners[key] = append(owners[key], i)
			}
		}
	}
	for _, indices := range owners {
		if len(indices) > 1 {
			for _, i := range indices {
				addReview(&snapshot.Facts[i], "competing_event_identifier")
				for j := range snapshot.Facts[i].Aliases {
					snapshot.Facts[i].Aliases[j].Verified = false
				}
			}
		}
	}
	for i := range snapshot.Facts {
		f := &snapshot.Facts[i]
		if len(f.Review) > 0 {
			for j := range f.Aliases {
				if strings.HasPrefix(f.Aliases[j].Namespace, "ufc.") {
					f.Aliases[j].Verified = false
				}
			}
		}
		if f.Status == "cancelled" || f.Status == "postponed" {
			for j := range f.Aliases {
				f.Aliases[j].Current = false
				f.Aliases[j].Verified = false
			}
		}
		if len(f.Review) > 0 {
			addDiagnostic(snapshot, Diagnostic{f.Name, f.Title, strings.Join(f.Review, ",")})
		}
	}
	return snapshot, nil
}

func sameBase(a, b Facts) bool {
	if a.Name != b.Name || a.Type != b.Type {
		return false
	}
	if a.Date == nil || b.Date == nil {
		return a.Date == nil && b.Date == nil
	}
	return *a.Date == *b.Date
}
