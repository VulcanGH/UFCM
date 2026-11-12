// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package matching

// TermGroup is a plain-language list of programming labels excluded for Main Card.
type TermGroup struct {
	Name  string   `json:"name"`
	Terms []string `json:"terms"`
}

// MainCardCheck explains exclusions independently of event identification.
type MainCardCheck struct {
	Status       string      `json:"status"`
	Reason       string      `json:"reason"`
	MatchedTerms []string    `json:"matched_terms"`
	Groups       []TermGroup `json:"groups"`
}

// MainCardTerms returns independent default phrase lists, without regular expressions.
func MainCardTerms() []TermGroup {
	return []TermGroup{
		{"Other event components", []string{"early prelims", "early preliminaries", "early preliminary", "early prelim", "earlyprelims", "earlyprelim", "prelims", "prelim", "preliminaries", "preliminary", "preliminary card", "preliminary fights", "undercard", "under card", "countdown", "embedded"}},
		{"Weigh-ins", []string{"weigh", "weigh in", "weigh ins", "weighin", "weighins", "weighing in", "weighing ins", "weighing ceremony"}},
		{"Studio and companion shows", []string{"pre show", "preshow", "post show", "postshow", "pre fight show", "prefight show", "post fight show", "postfight show", "preview show", "review show", "recap show", "weigh in show", "watch along", "watchalong", "watch party", "fight companion", "studio show", "live reaction", "live reactions"}},
		{"Press and media", []string{"press", "conference", "conferences", "media", "interview", "interviews", "scrum", "scrums", "media day", "media call", "conference call", "press conference", "post fight press conference", "postfight press conference", "pre fight press conference", "prefight press conference"}},
		{"Promotional material", []string{"promo", "promos", "promotional", "promotion", "trailer", "trailers", "teaser", "teasers", "preview", "previews", "cold open", "face off", "face offs", "faceoff", "faceoffs", "stare down", "staredown", "staredowns", "open workout", "open workouts", "workout", "workouts", "behind the scenes", "sneak peek"}},
		{"Highlights and individual fights", []string{"highlight", "highlights", "recap", "recaps", "review", "reviews", "analysis", "breakdown", "breakdowns", "prediction", "predictions", "free fight", "free fights", "full fight", "full fights", "fight replay", "fight replays", "best moments", "best finishes", "top finishes", "top knockouts", "top submissions", "fight motion", "slow motion", "walkout", "walkouts"}},
		{"Related programmes", []string{"road to the octagon", "inside the octagon", "thrill and the agony", "thrill the agony", "thrill and agony", "thrill agony", "ufc connected", "ultimate insider", "ufc unfiltered", "ufc unlocked", "ufc origins", "ufc chronicles", "ufc destined", "ufc journey", "ufc beyond the octagon", "ufc now", "ufc tonight", "ufc primetime", "ufc prime time", "bad blood", "fight flashback", "fight flashbacks", "lookin for a fight", "looking for a fight"}},
		{"Other competitions", []string{"the ultimate fighter", "ultimate fighter", "contender series", "dwcs", "road to ufc", "fight pass invitational", "ufc bjj", "ufc grappling", "zuffa boxing"}},
	}
}

// CheckMainCard matches whole phrases after separator/case normalization, without regex.
func CheckMainCard(title string, components Classification) MainCardCheck {
	return CheckMainCardWithTerms(title, components, nil)
}

// CheckMainCardWithTerms applies configured plain phrases; nil selects shipped defaults.
func CheckMainCardWithTerms(title string, components Classification, terms []string) MainCardCheck {
	result := MainCardCheck{Status: "needs review", Reason: "No explicit Main Card label; absence of excluded terms does not identify Main Card.", MatchedTerms: []string{}, Groups: MainCardTerms()}
	if terms != nil {
		result.Groups = []TermGroup{{Name: "Configured exclusions", Terms: terms}}
	}
	title = words(title)
	for _, group := range result.Groups {
		for _, term := range group.Terms {
			if contains(title, words(term)) {
				result.MatchedTerms = append(result.MatchedTerms, term)
			}
		}
	}
	if len(result.MatchedTerms) > 0 {
		result.Status = "excluded"
		result.Reason = "Excluded for Main Card by the matched programming terms. Event identity can still match."
		return result
	}
	if len(components.Values) == 1 && components.Values[0] == "main_card" {
		result.Status = "explicit label"
		result.Reason = "Explicit Main Card label with no excluded terms. This preview does not authorise acquisition."
	} else if len(components.Values) == 0 {
		result.Status = "inferred label"
		result.Reason = "Main Card can be inferred from a confident event match when no other component or excluded programming term is present."
	}
	return result
}
