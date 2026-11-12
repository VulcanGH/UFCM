// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package matching

// QualityCheck compares title resolution evidence with effective event preferences.
type QualityCheck struct {
	Matched bool     `json:"matched"`
	Allowed []string `json:"allowed"`
	Reason  string   `json:"reason"`
}

// CheckQuality requires exactly one accepted resolution and never silently falls back.
func CheckQuality(quality Classification, allowed []string) QualityCheck {
	result := QualityCheck{Allowed: append([]string{}, allowed...), Reason: "Resolution is unknown; review required"}
	resolutions := []string{}
	for _, value := range quality.Values {
		switch value {
		case "2160p", "1080p", "1080i", "720p", "480p":
			resolutions = append(resolutions, value)
		}
	}
	if len(resolutions) > 1 {
		result.Reason = "Conflicting resolution labels; review required"
		return result
	}
	if len(resolutions) == 1 {
		result.Reason = "Resolution " + resolutions[0] + " is not accepted for this event"
		for _, value := range allowed {
			if resolutions[0] == value {
				result.Matched = true
				result.Reason = "Resolution " + value + " matches the event preferences"
				break
			}
		}
	}
	return result
}
