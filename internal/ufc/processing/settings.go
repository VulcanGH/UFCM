// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package processing defines native copy-only processing preferences.
package processing

import (
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

// Settings are persistent preferences; saving them never starts processing.
type Settings struct {
	Revision             int64    `json:"revision"`
	SourceRoots          []string `json:"source_roots"`
	DestinationRoot      string   `json:"destination_root"`
	EventFolderTemplate  string   `json:"event_folder_template"`
	MainFileTemplate     string   `json:"main_file_template"`
	ExtraFileTemplate    string   `json:"extra_file_template"`
	ComponentFolder      string   `json:"component_folder"`
	ConflictPolicy       string   `json:"conflict_policy"`
	Extensions           []string `json:"extensions"`
	MaxConcurrent        int      `json:"max_concurrent"`
	MaxAttempts          int      `json:"max_attempts"`
	SABClientID          int32    `json:"sab_client_id"`
	SABCategory          string   `json:"sab_category"`
	AutoProcessCompleted bool     `json:"auto_process_completed"`
	CleanupSABDownload   bool     `json:"cleanup_sab_download"`
}

func cleanRoot(value string) (string, error) {
	value = filepath.Clean(strings.TrimSpace(value))
	if value == "." || !filepath.IsAbs(value) || len(value) > 4096 {
		return "", errors.Wrap(catalogue.ErrInvalidEdit, "roots must be absolute paths")
	}
	return value, nil
}

func overlaps(a, b string) bool {
	if runtime.GOOS == "windows" {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}

func validNameTemplate(value string) bool {
	if value == "" || len(value) > 256 || strings.ContainsAny(value, `/\:<>"|?*`) || strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") {
		return false
	}
	for _, token := range []string{"{{.EventTitle}}", "{{.Year}}", "{{.Component}}"} {
		value = strings.ReplaceAll(value, token, "X")
	}
	if strings.Contains(value, "{{") || strings.Contains(value, "}}") || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return false
	}
	name := strings.ToUpper(strings.SplitN(value, ".", 2)[0])
	if value == "." || value == ".." || name == "CON" || name == "PRN" || name == "AUX" || name == "NUL" || len(name) == 4 && (strings.HasPrefix(name, "COM") || strings.HasPrefix(name, "LPT")) && name[3] >= '1' && name[3] <= '9' {
		return false
	}
	return true
}

// Validate checks settings without accessing configured filesystem roots.
func Validate(s *Settings, protectedRoots, destinations []string) error {
	if s.Revision < 1 || len(s.SourceRoots) > 8 {
		return errors.Wrap(catalogue.ErrInvalidEdit, "invalid processing revision or source roots")
	}
	if !validNameTemplate(s.EventFolderTemplate) || !strings.Contains(s.EventFolderTemplate, "{{.EventTitle}}") || !strings.Contains(s.EventFolderTemplate, "{{.Year}}") || !validNameTemplate(s.MainFileTemplate) || !strings.Contains(s.MainFileTemplate, "{{.EventTitle}}") || !validNameTemplate(s.ExtraFileTemplate) || !strings.Contains(s.ExtraFileTemplate, "{{.Component}}") || !validNameTemplate(s.ComponentFolder) || strings.Contains(s.ComponentFolder, "{{") {
		return errors.Wrap(catalogue.ErrInvalidEdit, "invalid processing naming template")
	}
	if s.ConflictPolicy != "review" && s.ConflictPolicy != "skip" && s.ConflictPolicy != "keep_both" {
		return errors.Wrap(catalogue.ErrInvalidEdit, "invalid conflict policy")
	}
	if s.MaxConcurrent < 1 || s.MaxConcurrent > 4 || s.MaxAttempts < 1 || s.MaxAttempts > 10 || s.SABClientID < 0 || len(s.SABCategory) > 128 || strings.IndexFunc(s.SABCategory, unicode.IsControl) >= 0 {
		return errors.Wrap(catalogue.ErrInvalidEdit, "invalid processing preference")
	}
	if len(s.Extensions) < 1 || len(s.Extensions) > 10 {
		return errors.Wrap(catalogue.ErrInvalidEdit, "choose supported media extensions")
	}
	seen := map[string]bool{}
	for _, ext := range s.Extensions {
		if ext == "" || ext[0] != '.' || len(ext) > 12 || strings.IndexFunc(ext[1:], func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) >= 0 || seen[strings.ToLower(ext)] {
			return errors.Wrap(catalogue.ErrInvalidEdit, "invalid or duplicate media extension")
		}
		seen[strings.ToLower(ext)] = true
	}
	if s.DestinationRoot != "" {
		root, err := cleanRoot(s.DestinationRoot)
		if err != nil {
			return err
		}
		matched := false
		for _, libraryRoot := range destinations {
			other := filepath.Clean(libraryRoot)
			if runtime.GOOS == "windows" {
				matched = matched || strings.EqualFold(root, other)
			} else {
				matched = matched || root == other
			}
		}
		if !matched {
			return errors.Wrap(catalogue.ErrInvalidEdit, "destination must be a configured library root")
		}
		s.DestinationRoot = root
	}
	for i, value := range s.SourceRoots {
		root, err := cleanRoot(value)
		if err != nil {
			return err
		}
		for _, libraryRoot := range protectedRoots {
			if overlaps(root, libraryRoot) {
				return errors.Wrap(catalogue.ErrInvalidEdit, "source roots must not overlap library roots")
			}
		}
		for j := 0; j < i; j++ {
			if overlaps(root, s.SourceRoots[j]) {
				return errors.Wrap(catalogue.ErrInvalidEdit, "source roots must not overlap")
			}
		}
		s.SourceRoots[i] = root
	}
	if (s.AutoProcessCompleted || s.CleanupSABDownload) && (s.SABClientID == 0 || s.DestinationRoot == "" || len(s.SourceRoots) == 0) {
		return errors.Wrap(catalogue.ErrInvalidEdit, "choose a SAB client, completed download folder and library destination before enabling automatic processing or cleanup")
	}
	return nil
}
