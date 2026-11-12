// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/matching"
	"github.com/autobrr/autobrr/pkg/errors"
)

// FilePlan describes one read-only source observation and proposed destination.
type FilePlan struct {
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Component   string   `json:"component"`
	Size        int64    `json:"size"`
	State       string   `json:"state"`
	Reasons     []string `json:"reasons"`
}

// Preview describes a proposed import without creating a job or changing files.
type Preview struct {
	AcquisitionID   string     `json:"acquisition_id"`
	EventID         string     `json:"event_id"`
	EventTitle      string     `json:"event_title"`
	DestinationRoot string     `json:"destination_root"`
	Status          string     `json:"status"`
	Reasons         []string   `json:"reasons"`
	Files           []FilePlan `json:"files"`
}

func within(root, target string) bool {
	root, target = filepath.Clean(root), filepath.Clean(target)
	if runtime.GOOS == "windows" {
		root, target = strings.ToLower(root), strings.ToLower(target)
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func verifiedNumber(event catalogue.Event, namespace string) string {
	number := ""
	for _, alias := range event.Aliases {
		if alias.Namespace == namespace && alias.Current && alias.Verified {
			if number != "" && number != alias.Value {
				return ""
			}
			number = alias.Value
		}
	}
	return number
}

func eventTitle(event catalogue.Event) (string, []string) {
	reasons := []string{}
	prefix := ""
	switch event.Values.Type {
	case "numbered":
		if number := verifiedNumber(event, "ufc.numbered"); number != "" {
			prefix = "UFC " + number
		}
	case "fight_night":
		for _, option := range []struct{ namespace, label string }{{"ufc.fight_night", "UFC Fight Night "}, {"ufc.espn", "UFC on ESPN "}, {"ufc.espn_plus", "UFC on ESPN+ "}, {"ufc.abc", "UFC on ABC "}, {"ufc.freedom", "UFC Freedom "}} {
			if number := verifiedNumber(event, option.namespace); number != "" {
				prefix = option.label + number
				break
			}
		}
	}
	usingFallback := prefix == ""
	if usingFallback {
		prefix = event.Values.Name
		reasons = append(reasons, "Verified event number is unavailable; review the event title")
	}
	if len(event.Values.Headliners) == 2 && strings.TrimSpace(event.Values.Headliners[0]) != "" && strings.TrimSpace(event.Values.Headliners[1]) != "" {
		if !usingFallback {
			prefix += " " + strings.TrimSpace(event.Values.Headliners[0]) + " vs. " + strings.TrimSpace(event.Values.Headliners[1])
		}
	} else if event.Values.Status != "completed" || !strings.Contains(event.Values.Name, ":") {
		reasons = append(reasons, "Headlining fighters are unavailable; wait or correct the event before publishing")
	} else if !usingFallback {
		prefix = event.Values.Name
	}
	if event.Values.Date == nil || len(*event.Values.Date) < 4 {
		reasons = append(reasons, "Event year is unavailable")
		return prefix, reasons
	}
	return prefix, reasons
}

func portableName(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return ' '
		}
		return r
	}, value)
	value = strings.TrimRight(strings.Join(strings.Fields(value), " "), ". ")
	if !validNameTemplate(value) {
		return ""
	}
	return value
}

func renderName(pattern string, eventTitle string, year string, component string) string {
	value := strings.ReplaceAll(pattern, "{{.EventTitle}}", eventTitle)
	value = strings.ReplaceAll(value, "{{.Year}}", year)
	value = strings.ReplaceAll(value, "{{.Component}}", component)
	return portableName(value)
}

func componentLabel(key string) string {
	switch key {
	case "main_card":
		return "Main Card"
	case "prelims":
		return "Prelims"
	case "early_prelims":
		return "Early Prelims"
	case "countdown":
		return "Countdown"
	case "embedded":
		return "Embedded"
	case "weigh_ins":
		return "Weigh-ins"
	default:
		return ""
	}
}

func sourceFiles(ctx context.Context, storage, resolvedRoot string, extensions []string) ([]FilePlan, error) {
	allowed := map[string]bool{}
	for _, ext := range extensions {
		allowed[strings.ToLower(ext)] = true
	}
	info, err := os.Lstat(storage)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "completed download path is unavailable or unsafe")
	}
	files := []FilePlan{}
	count := 0
	add := func(path string, info fs.FileInfo) error {
		if !info.Mode().IsRegular() || !allowed[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		if len(files) >= 64 {
			return errors.Wrap(catalogue.ErrInvalidEdit, "too many media files for one preview")
		}
		rel := filepath.Base(path)
		if info.IsDir() {
			return nil
		}
		if path != storage {
			rel, err = filepath.Rel(storage, path)
			if err != nil {
				return err
			}
		}
		if info.Size() == 0 {
			return errors.Wrap(catalogue.ErrInvalidEdit, "completed download contains an empty media file")
		}
		files = append(files, FilePlan{Source: filepath.ToSlash(rel), Size: info.Size(), State: "planned", Reasons: []string{}})
		return nil
	}
	if info.Mode().IsRegular() {
		if err := add(storage, info); err != nil {
			return nil, err
		}
	} else if info.IsDir() {
		err = filepath.WalkDir(storage, func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				return errors.Wrap(catalogue.ErrInvalidEdit, "completed download cannot be fully inspected")
			}
			count++
			if count > 2000 {
				return errors.Wrap(catalogue.ErrInvalidEdit, "too many entries for one preview")
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.Wrap(catalogue.ErrInvalidEdit, "completed download contains a linked path")
			}
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil || !within(resolvedRoot, resolved) {
				return errors.Wrap(catalogue.ErrInvalidEdit, "completed download contains an unsafe path")
			}
			if path != storage && strings.HasPrefix(entry.Name(), "_") {
				return errors.Wrap(catalogue.ErrInvalidEdit, "temporary paths remain in completed download")
			}
			if entry.IsDir() {
				rel, err := filepath.Rel(storage, path)
				if err != nil {
					return err
				}
				if len(strings.Split(rel, string(filepath.Separator))) > 8 {
					return errors.Wrap(catalogue.ErrInvalidEdit, "completed download is too deeply nested")
				}
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return errors.Wrap(catalogue.ErrInvalidEdit, "completed download cannot be fully inspected")
			}
			return add(path, info)
		})
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "completed download is not a regular file or folder")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Source < files[j].Source })
	return files, nil
}

func destinationState(root, relative string) string {
	current := root
	for _, part := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return "planned"
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return "unsafe"
		}
	}
	return "conflict"
}

// Preview reads a completed UFC acquisition and filesystem metadata without mutations.
func (s *Service) Preview(ctx context.Context, id string) (*Preview, error) {
	if id == "" || len(id) > 128 {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "choose a tracked UFC download")
	}
	job, err := s.repo.Acquisition(ctx, id)
	if err != nil {
		return nil, err
	}
	if job.State != "completed" || job.RemoteID == "" || job.Storage == "" {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "wait for confirmed SAB completion before previewing")
	}
	settings, err := s.repo.ProcessingSettings(ctx)
	if err != nil {
		return nil, err
	}
	view, err := s.repo.LibraryView(ctx)
	if err != nil {
		return nil, err
	}
	roots, destinations := []string{}, []string{}
	for _, root := range view.Settings.Roots {
		roots = append(roots, root.Path)
		if root.Enabled {
			destinations = append(destinations, root.Path)
		}
	}
	if err := Validate(settings, roots, destinations); err != nil {
		return nil, err
	}
	if settings.DestinationRoot == "" || len(settings.SourceRoots) == 0 {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "configure completed download and library folders first")
	}
	storage, err := filepath.Abs(job.Storage)
	if err != nil || !filepath.IsAbs(job.Storage) {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "SAB completed storage path is invalid")
	}
	allowed := false
	selectedRoot := ""
	for _, root := range settings.SourceRoots {
		if within(root, storage) {
			allowed = true
			selectedRoot = root
			break
		}
	}
	if !allowed {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "SAB completed storage is outside configured source folders")
	}
	resolved, err := filepath.EvalSymlinks(storage)
	if err != nil {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "SAB completed storage is unavailable")
	}
	resolvedSourceRoot, err := filepath.EvalSymlinks(selectedRoot)
	if err != nil || !within(resolvedSourceRoot, resolved) {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "SAB completed storage escapes its configured source folder")
	}
	for _, root := range roots {
		if overlaps(storage, root) || overlaps(resolved, root) {
			return nil, errors.Wrap(catalogue.ErrInvalidEdit, "SAB storage overlaps a protected library folder")
		}
		resolvedLibrary, err := filepath.EvalSymlinks(root)
		if err == nil && overlaps(resolved, resolvedLibrary) {
			return nil, errors.Wrap(catalogue.ErrInvalidEdit, "SAB storage overlaps a protected library folder")
		}
	}
	event, err := s.repo.LoadCatalogueEvent(ctx, job.EventID)
	if err != nil {
		return nil, err
	}
	if event.Corrections.Deleted {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "event was removed; review the download assignment")
	}
	result := &Preview{AcquisitionID: id, EventID: event.ID, DestinationRoot: settings.DestinationRoot, Status: "ready", Reasons: []string{}, Files: []FilePlan{}}
	title, titleReasons := eventTitle(event)
	result.EventTitle = title
	result.Reasons = append(result.Reasons, titleReasons...)
	if len(event.Review) > 0 {
		result.Reasons = append(result.Reasons, "Event catalogue entry needs review")
	}
	if view.Settings.Running {
		result.Reasons = append(result.Reasons, "Library scan is running")
	}
	for _, root := range view.Settings.Roots {
		if root.Enabled && root.State != "available" {
			result.Reasons = append(result.Reasons, "Library location is unscanned or unavailable")
			break
		}
	}
	for _, group := range view.Groups {
		if group.EventID == event.ID && group.Files > 0 {
			if group.Components[job.Component] == "present" || group.Components[job.Component] == "review" || group.Components[job.Component] == "unavailable" || group.Locations > 1 || group.Review > 0 {
				result.Reasons = append(result.Reasons, "Existing library files for this event need folder review")
			}
			break
		}
	}
	if job.Component == "" || componentLabel(job.Component) == "" {
		result.Reasons = append(result.Reasons, "Download component is unsupported")
	}
	files, err := sourceFiles(ctx, storage, resolvedSourceRoot, settings.Extensions)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		result.Reasons = append(result.Reasons, "No supported media files were found")
	}
	if info, err := os.Lstat(settings.DestinationRoot); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		result.Reasons = append(result.Reasons, "Library destination is unavailable or linked")
	}
	preferences, err := s.repo.CataloguePreferences(ctx)
	if err != nil {
		return nil, err
	}
	year := ""
	if event.Values.Date != nil && len(*event.Values.Date) >= 4 {
		year = (*event.Values.Date)[:4]
	}
	folder := renderName(settings.EventFolderTemplate, title, year, "")
	componentCounts := map[string]int{}
	selectedComponentFound := false
	for i := range files {
		file := &files[i]
		if match := matching.MatchLibrary(filepath.Base(storage)+" "+file.Source, []catalogue.Event{event}); match.Status != "matched" {
			file.Reasons = append(file.Reasons, "File and folder names do not confidently identify the assigned event")
		}
		classification := matching.ClassifyComponents(file.Source)
		if len(classification.Values) == 1 {
			file.Component = classification.Values[0]
		} else if len(classification.Values) == 0 && matching.CheckMainCardWithTerms(file.Source, classification, preferences.MainCardTerms).Status == "inferred label" && !strings.Contains(strings.ToLower(file.Source), "featurettes") {
			file.Component = "main_card"
		} else {
			file.Reasons = append(file.Reasons, "Filename component is ambiguous or excluded")
		}
		if len(files) == 1 && file.Component != job.Component {
			file.Reasons = append(file.Reasons, "Filename component conflicts with the selected download component")
		}
		if file.Component == "main_card" && matching.CheckMainCardWithTerms(file.Source, classification, preferences.MainCardTerms).Status == "excluded" {
			file.Reasons = append(file.Reasons, "Filename has programming excluded from Main Card")
		}
		if event.Values.Type == "fight_night" && file.Component == "early_prelims" {
			file.Reasons = append(file.Reasons, "Fight Night Early Prelims need review")
		}
		if event.Quality != nil && len(event.Quality.Effective) > 0 {
			resolutions := 0
			for _, value := range matching.AssessQuality(file.Source).Values {
				if slices.Contains([]string{"2160p", "1080p", "1080i", "720p", "480p"}, value) {
					resolutions++
					if !slices.Contains(event.Quality.Effective, value) {
						file.Reasons = append(file.Reasons, "Filename resolution differs from this event's accepted resolutions")
					}
				}
			}
			if resolutions > 1 {
				file.Reasons = append(file.Reasons, "Filename asserts multiple resolutions")
			}
		}
		if file.Component == job.Component {
			selectedComponentFound = true
		}
		if file.Component != "" {
			componentCounts[file.Component]++
		}
	}
	if !selectedComponentFound {
		result.Reasons = append(result.Reasons, "No file matches the selected download component")
	}
	componentParts := map[string]int{}
	for i := range files {
		file := &files[i]
		pattern := settings.ExtraFileTemplate
		rel := filepath.Join(folder, settings.ComponentFolder)
		if file.Component == "main_card" {
			pattern = settings.MainFileTemplate
			rel = folder
		}
		name := renderName(pattern, title, year, componentLabel(file.Component))
		if folder == "" || name == "" {
			file.Reasons = append(file.Reasons, "Proposed name is invalid")
		}
		if componentCounts[file.Component] > 1 {
			componentParts[file.Component]++
			name += " - Part " + strconv.Itoa(componentParts[file.Component])
		}
		file.Destination = filepath.ToSlash(filepath.Join(rel, name+filepath.Ext(file.Source)))
		if state := destinationState(settings.DestinationRoot, file.Destination); state != "planned" {
			file.Reasons = append(file.Reasons, "Destination exists or contains an unsafe path; content is not verified")
		}
		if len(file.Reasons) > 0 {
			file.State = "review"
		}
	}
	result.Files = files
	if len(result.Reasons) > 0 {
		result.Status = "needs_review"
	}
	for _, file := range files {
		if file.State != "planned" {
			result.Status = "needs_review"
		}
	}
	return result, nil
}
