// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package library

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/matching"
)

// Key gives stable local observation identity without reading media bytes.
func Key(value string) string { sum := sha256.Sum256([]byte(value)); return hex.EncodeToString(sum[:]) }

func folderGroup(root, relative string) string {
	return "folder:" + Key(root+"\x00"+path.Dir(relative))
}

// Identify combines folder/file evidence and keeps conflicting identities for review.
func Identify(file File, events []catalogue.Event, cache map[string]matching.Preview) File {
	parts := strings.Split(file.Path, "/")
	parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], path.Ext(parts[len(parts)-1]))
	year := ""
	conflictingYears := false
	for _, part := range parts {
		if value := matching.LibraryYear(part); value != "" {
			if year != "" && year != value {
				conflictingYears = true
			}
			year = value
		}
	}
	ids := map[string]bool{}
	uncertain := false
	problems := []string{}
	rootName := filepath.Base(file.RootPath)
	rootMatch, ok := cache[rootName]
	if !ok {
		rootMatch = matching.MatchLibrary(rootName, events)
		cache[rootName] = rootMatch
	}
	if rootMatch.Status == "matched" && len(rootMatch.Candidates) == 1 {
		ids[rootMatch.Candidates[0].ID] = true
	}
	for _, part := range parts {
		title := part
		if year != "" && !conflictingYears && matching.LibraryYear(part) == "" {
			title += " (" + year + ")"
		}
		p, ok := cache[title]
		if !ok {
			p = matching.MatchLibrary(title, events)
			cache[title] = p
		}
		if p.Status == "matched" && len(p.Candidates) == 1 {
			ids[p.Candidates[0].ID] = true
		} else if len(p.Identifiers) > 0 || len(p.Candidates) > 0 {
			uncertain = true
			if problem := identityProblem(p); len(problems) < 3 && !slices.Contains(problems, problem) {
				problems = append(problems, problem)
			}
		}
	}
	file.State = "review"
	file.Reason = "Event identity is unknown or ambiguous"
	file.Component = "unknown"
	file.Resolution = "unknown"
	text := strings.Join(parts, " ")
	components := matching.ClassifyComponents(text)
	quality := matching.AssessQuality(text)
	if len(components.Values) == 1 {
		file.Component = components.Values[0]
	}
	if quality.Status != "conflicting resolutions" {
		for _, value := range quality.Values {
			switch value {
			case "2160p", "1080p", "1080i", "720p", "480p":
				file.Resolution = value
			}
		}
	}
	file.Group = folderGroup(file.RootID, file.Path)
	if len(ids) == 1 {
		for id := range ids {
			file.EventID = id
		}
		file.Group = "event:" + file.EventID
	}
	if len(ids) != 1 || uncertain {
		if len(ids) > 1 {
			file.Reason = "Folder and filename identify different events"
		} else if len(problems) > 0 {
			file.Reason = strings.Join(problems, "; ")
		}
		return detected(file)
	}
	for _, part := range parts {
		year := matching.LibraryYear(part)
		if year != "" {
			for _, e := range events {
				if e.ID == file.EventID && e.Values.Date != nil && len(*e.Values.Date) >= 4 && year != (*e.Values.Date)[:4] {
					file.Reason = "Folder year conflicts with catalogue date"
					return detected(file)
				}
			}
		}
	}
	check := matching.CheckMainCard(text, components)
	if len(components.Values) == 0 {
		extraFolder := false
		for _, part := range parts[:len(parts)-1] {
			p := strings.ToLower(part)
			if p == "featurettes" || p == "extras" || p == "specials" || p == "bonus" {
				extraFolder = true
			}
		}
		if check.Status == "excluded" || extraFolder {
			file.State = "extra"
			file.Component = "other"
			file.Reason = "Supplementary programming does not establish card coverage"
			return detected(file)
		}
		file.Component = "main_card"
	} else if len(components.Values) == 1 {
		file.Component = components.Values[0]
	} else {
		file.Reason = "Several component labels appear in the path"
		return detected(file)
	}
	if file.Component == "main_card" && check.Status == "excluded" {
		file.State = "extra"
		file.Reason = "Main Card exclusion: " + strings.Join(check.MatchedTerms, ", ")
		return detected(file)
	}
	if quality.Status == "conflicting resolutions" {
		file.Reason = "Conflicting resolution labels"
		return detected(file)
	}
	file.State = "confirmed"
	file.Reason = "Identified from folder/filename; resolution is a title assertion, not a media probe"
	if file.Size == 0 {
		file.State = "review"
		file.Reason = "Empty file; coverage is not confirmed"
	}
	return detected(file)
}

func identityProblem(preview matching.Preview) string {
	if len(preview.Candidates) == 0 {
		for _, note := range preview.Notes {
			if strings.HasPrefix(note, "No saved numbered event has the identity ") {
				return note + "; the catalogue needs this event before it can be identified"
			}
		}
		return "No saved catalogue alias identifies this title: " + preview.Title
	}
	if len(preview.Candidates) > 1 {
		return "Several catalogue events match this title: " + preview.Title
	}
	candidate := preview.Candidates[0]
	if year := matching.LibraryYear(preview.Title); year != "" && candidate.Date != nil && len(*candidate.Date) >= 4 && year != (*candidate.Date)[:4] {
		return "Folder/filename year " + year + " differs from " + candidate.Name + " (" + (*candidate.Date)[:4] + ")"
	}
	return candidate.Name + ": " + strings.Join(candidate.Conflicts, ", ")
}

func detected(f File) File {
	f.DetectedEvent = f.EventID
	f.DetectedComponent = f.Component
	f.DetectedResolution = f.Resolution
	f.DetectedState = f.State
	f.DetectedReason = f.Reason
	return f
}
