package catalogue

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
)

type AliasKey struct {
	Namespace string `json:"namespace"`
	Value     string `json:"value"`
}

func KeyForAlias(a Alias) AliasKey { return AliasKey{a.Namespace, Normalize(a.Value)} }

type ReviewRecord struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Revision int64   `json:"revision"`
	Date     *string `json:"date,omitempty"`
}

type AliasConflict struct {
	AliasKey
	Records []ReviewRecord `json:"records"`
}

type ReviewChoice struct {
	AliasKey
	Records    []ReviewRecord `json:"records"`
	Keep       string         `json:"keep"`
	FromSource bool           `json:"-"`
}

func ConflictingAliases(events []Event, id string) []AliasConflict {
	owners := map[AliasKey][]ReviewRecord{}
	var aliases []AliasKey
	for _, event := range events {
		if event.Corrections.Deleted {
			continue
		}
		for _, alias := range event.Aliases {
			if !alias.Current || alias.Namespace == "title" {
				continue
			}
			key := KeyForAlias(alias)
			if event.ID == id {
				aliases = append(aliases, key)
			}
			owners[key] = append(owners[key], ReviewRecord{event.ID, event.Values.Name, event.Revision, event.Values.Date})
		}
	}
	result := []AliasConflict{}
	for _, key := range aliases {
		if len(owners[key]) > 1 {
			slices.SortFunc(owners[key], func(a, b ReviewRecord) int { return strings.Compare(a.ID, b.ID) })
			result = append(result, AliasConflict{key, owners[key]})
		}
	}
	return result
}

type conflictEditor interface {
	ResolveAliasConflict(context.Context, string, ReviewChoice, string) error
}

func (s *Service) AliasConflicts(ctx context.Context, id string) ([]AliasConflict, error) {
	events, err := s.MatchingEvents(ctx)
	if err != nil {
		return nil, err
	}
	return ConflictingAliases(events, id), nil
}

func (s *Service) ResolveAliasConflict(ctx context.Context, id string, choice ReviewChoice, actor string) error {
	repo, ok := s.repo.(conflictEditor)
	if !ok {
		return ErrInvalidEdit
	}
	return repo.ResolveAliasConflict(ctx, id, choice, actor)
}

type reviewSource interface {
	ReadReview(context.Context, []Facts, AliasKey) ([]Facts, error)
}

func (s *Service) RecheckAliasConflict(ctx context.Context, id string, choice ReviewChoice, actor string) error {
	source, ok := s.source.(reviewSource)
	if !ok {
		return ErrInvalidEdit
	}
	events, err := s.MatchingEvents(ctx)
	if err != nil {
		return err
	}
	var known []Facts
	locals := map[int64]string{}
	for _, e := range events {
		if !slices.ContainsFunc(choice.Records, func(r ReviewRecord) bool { return r.ID == e.ID }) {
			continue
		}
		if e.Source.PageID == 0 {
			return fmt.Errorf("%w: a local record is involved. Choose which record keeps the alias", ErrInvalidEdit)
		}
		known = append(known, e.Source)
		locals[e.Source.PageID] = e.ID
	}
	if len(known) < 2 || len(known) > 32 || len(known) != len(choice.Records) {
		return ErrRevisionConflict
	}
	fresh, err := source.ReadReview(ctx, known, choice.AliasKey)
	if err != nil {
		return err
	}
	choice.Keep = ""
	for _, f := range fresh {
		for _, a := range f.Aliases {
			if !a.Current || KeyForAlias(a) != choice.AliasKey {
				continue
			}
			if choice.Keep != "" || !a.Verified || locals[f.PageID] == "" {
				return fmt.Errorf("%w: the source still does not identify one verified owner. Choose an owner to resolve this clash", ErrInvalidEdit)
			}
			choice.Keep = locals[f.PageID]
		}
	}
	if choice.Keep == "" {
		return fmt.Errorf("%w: the source does not confirm this alias. Saved claims were kept", ErrInvalidEdit)
	}
	choice.FromSource = true
	return s.ResolveAliasConflict(ctx, id, choice, actor)
}

var reviewFields = map[string]string{
	"article_list_date_conflict":                  "date",
	"unrecognised_date":                           "date",
	"source_date_unknown_previous_retained":       "date",
	"article_list_headliner_conflict":             "headliners",
	"source_headliners_unknown_previous_retained": "headliners",
	"article_list_type_conflict":                  "type",
}

func addReviewEvidence(f *Facts, reason string, evidence ...string) {
	if f.ReviewEvidence == nil {
		f.ReviewEvidence = map[string][]string{}
	} else {
		f.ReviewEvidence = maps.Clone(f.ReviewEvidence)
	}
	f.ReviewEvidence[reason] = append(append([]string{}, f.ReviewEvidence[reason]...), evidence...)
}

func reviewFingerprint(f Facts, reason string) string {
	evidence := append([]string{}, f.ReviewEvidence[reason]...)
	if len(evidence) == 0 {
		evidence = []string{strconv.FormatInt(f.PageID, 10)}
		switch reviewFields[reason] {
		case "date":
			if f.Date != nil {
				evidence = append(evidence, *f.Date)
			}
		case "headliners":
			evidence = append(evidence, f.Headliners...)
		case "type":
			evidence = append(evidence, f.Type)
		}
		if strings.HasPrefix(reason, "article_list_") {
			evidence = append(evidence, strconv.FormatInt(f.ArticleRevision, 10))
		}
	}
	slices.Sort(evidence)
	raw, _ := json.Marshal(slices.Compact(evidence))
	return string(raw)
}

func sourceReviewEvidence(event Event, field string) map[string]string {
	resolved := map[string]string{}
	for _, reason := range event.Review {
		if reviewFields[reason] == field {
			resolved[reason] = reviewFingerprint(event.Source, reason)
		}
	}
	return resolved
}

func (e *Event) resolveSourceReviews() {
	resolved := make([]string, 0, len(e.Review))
	for _, reason := range e.Review {
		var correction *Correction
		switch reviewFields[reason] {
		case "date":
			correction = e.Corrections.Date
		case "headliners":
			correction = e.Corrections.Headliners
		case "type":
			correction = e.Corrections.Type
		}
		if correction != nil && correction.Protected && correction.Evidence[reason] == reviewFingerprint(e.Source, reason) {
			continue
		}
		resolved = append(resolved, reason)
	}
	e.Review = resolved
}

type AliasDecision struct {
	Alias Alias     `json:"alias"`
	Keep  bool      `json:"keep"`
	Actor string    `json:"actor"`
	At    time.Time `json:"at"`
}

func (e *Event) DecideAlias(key AliasKey, keep bool, actor string, at time.Time) {
	for _, a := range e.Aliases {
		if a.Current && KeyForAlias(a) == key {
			a.Manual, a.Verified = true, true
			decisions := slices.Clone(e.Corrections.AliasDecisions)
			e.Corrections.AliasDecisions = append(slices.DeleteFunc(decisions, func(d AliasDecision) bool {
				return KeyForAlias(d.Alias) == key
			}), AliasDecision{a, keep, actor, at.UTC()})
			e.applyAliasDecisions()
			return
		}
	}
}

func (e *Event) applyAliasDecisions() {
	for _, decision := range e.Corrections.AliasDecisions {
		e.Aliases = slices.DeleteFunc(e.Aliases, func(a Alias) bool {
			return a.Current && KeyForAlias(a) == KeyForAlias(decision.Alias)
		})
		if decision.Keep {
			e.Aliases = retainAlias(e.Aliases, decision.Alias)
		}
	}
}

func (e *Event) retainUnchangedDecisions(old Event) {
	e.Corrections.AliasDecisions = slices.DeleteFunc(slices.Clone(e.Corrections.AliasDecisions), func(d AliasDecision) bool {
		find := func(aliases []Alias) (Alias, bool) {
			for _, a := range aliases {
				if a.Current && KeyForAlias(a) == KeyForAlias(d.Alias) {
					return a, true
				}
			}
			return Alias{}, false
		}
		before, had := find(old.Aliases)
		after, has := find(e.Aliases)
		return has != had || (has && before.Verified != after.Verified)
	})
}
