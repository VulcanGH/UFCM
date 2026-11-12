// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package redact removes configured webhook credentials from diagnostics.
package redact

import (
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
)

// Text masks the configured URL and credential components echoed by a remote server.
func Text(text, endpoint string) string {
	if endpoint == "" {
		return text
	}
	secrets := []string{endpoint}
	if parsed, err := url.Parse(endpoint); err == nil {
		if parsed.User != nil {
			secrets = append(secrets, parsed.User.Username())
		}
		if password, set := parsed.User.Password(); set {
			secrets = append(secrets, password)
		}
		segments := strings.Split(strings.TrimRight(parsed.Path, "/"), "/")
		if len(segments) > 0 {
			secrets = append(secrets, segments[len(segments)-1])
		}
		for _, values := range parsed.Query() {
			secrets = append(secrets, values...)
		}
	}
	forms := make(map[string]struct{})
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		encoded, _ := json.Marshal(secret)
		for _, form := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret), string(encoded[1 : len(encoded)-1])} {
			forms[form] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(forms))
	for form := range forms {
		ordered = append(ordered, form)
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	replacements := make([]string, 0, 2*len(ordered))
	for _, form := range ordered {
		replacements = append(replacements, form, "<redacted>")
	}
	return strings.NewReplacer(replacements...).Replace(text)
}

type safeError struct {
	cause   error
	message string
}

func (e safeError) Error() string        { return e.message }
func (e safeError) Is(target error) bool { return errors.Is(e.cause, target) }

// Error masks credentials while preserving errors.Is without exposing an unsafe unwrap chain.
func Error(err error, endpoint string) error {
	if err == nil {
		return nil
	}
	return safeError{cause: err, message: Text(err.Error(), endpoint)}
}
