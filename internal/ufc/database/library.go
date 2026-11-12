// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/library"
	"github.com/autobrr/autobrr/pkg/errors"
)

func librarySettings(ctx context.Context, q queryer) (*library.Settings, error) {
	s := &library.Settings{Roots: []library.Root{}}
	if err := q.QueryRowContext(ctx, "SELECT revision,running,run_id,last_scan,message FROM ufc_library_settings WHERE id=1").Scan(&s.Revision, &s.Running, &s.RunID, &s.LastScan, &s.Message); err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, "SELECT id,path,enabled,state,message FROM ufc_library_roots ORDER BY path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r library.Root
		if err := rows.Scan(&r.ID, &r.Path, &r.Enabled, &r.State, &r.Message); err != nil {
			return nil, err
		}
		s.Roots = append(s.Roots, r)
	}
	return s, rows.Err()
}

// LibrarySettings reads persistent roots and scan state from one snapshot.
func (r *Repository) LibrarySettings(ctx context.Context) (*library.Settings, error) {
	var s *library.Settings
	err := r.readTransaction(ctx, func(tx *sql.Tx) error { var err error; s, err = librarySettings(ctx, tx); return err })
	return s, err
}

// ClearLibrary forgets inventory atomically without accessing any filesystem paths.
func (r *Repository) ClearLibrary(ctx context.Context, revision int64) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE ufc_library_settings SET revision=revision+1,last_scan=NULL,run_id='',message='Saved library inventory cleared; scan to rebuild coverage' WHERE id=1 AND revision=$1 AND running=FALSE", revision)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return catalogue.ErrRevisionConflict
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM ufc_library_files"); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE ufc_library_roots SET state='unscanned',message='Saved inventory cleared; scan required' WHERE enabled=TRUE")
		return err
	})
}

// SaveLibraryRoots replaces configured paths without deleting retained inventory.
func (r *Repository) SaveLibraryRoots(ctx context.Context, revision int64, roots []library.Root) (*library.Settings, error) {
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE ufc_library_settings SET revision=revision+1 WHERE id=1 AND revision=$1 AND running=FALSE", revision)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return catalogue.ErrRevisionConflict
		}
		_, err = tx.ExecContext(ctx, "UPDATE ufc_library_roots SET enabled=FALSE,state='retired',message='Removed from configured roots; inventory retained' WHERE enabled=TRUE")
		if err != nil {
			return err
		}
		for _, root := range roots {
			_, err = tx.ExecContext(ctx, `INSERT INTO ufc_library_roots(id,path,enabled,state) VALUES($1,$2,TRUE,'unscanned') ON CONFLICT(id) DO UPDATE SET path=excluded.path,enabled=TRUE,state='unscanned',message=''`, root.ID, root.Path)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.LibrarySettings(ctx)
}

// BeginLibraryScan claims a durable single scan before work starts.
func (r *Repository) BeginLibraryScan(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "UPDATE ufc_library_settings SET running=TRUE,run_id=$1,message='Scanning library',revision=revision+1 WHERE id=1 AND running=FALSE", id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.Wrap(catalogue.ErrInvalidEdit, "a library scan is already running")
	}
	return nil
}

// FinishLibraryScan clears running state; interrupted work never infers removals.
func (r *Repository) FinishLibraryScan(ctx context.Context, id, message string, interrupted bool) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE ufc_library_settings SET running=FALSE,message=$1,last_scan=$2,revision=revision+1 WHERE id=1 AND run_id=$3 AND running=TRUE", message, time.Now().UTC().Format(time.RFC3339Nano), id)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if interrupted {
			_, err = tx.ExecContext(ctx, "UPDATE ufc_library_roots SET state='unavailable',message='Scan interrupted; saved inventory retained' WHERE enabled=TRUE")
		}
		return err
	})
}

// CommitLibraryRoot merges one complete observation set and retains manual bindings.
func (r *Repository) CommitLibraryRoot(ctx context.Context, run string, root library.Root, files []library.File, complete bool) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE ufc_library_settings SET revision=revision+1 WHERE id=1 AND running=TRUE AND run_id=$1", run)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return catalogue.ErrRevisionConflict
		}
		for _, f := range files {
			_, err = tx.ExecContext(ctx, `INSERT INTO ufc_library_files(id,root_id,relative_path,size,modified,event_id,component,resolution,state,reason,present,group_key,last_seen,detected_event,detected_component,detected_resolution,detected_state,detected_reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,TRUE,$11,$12,$6,$7,$8,$9,$10) ON CONFLICT(id) DO UPDATE SET revision=ufc_library_files.revision+1,size=excluded.size,modified=excluded.modified,present=TRUE,last_seen=excluded.last_seen,detected_event=excluded.detected_event,detected_component=excluded.detected_component,detected_resolution=excluded.detected_resolution,detected_state=excluded.detected_state,detected_reason=excluded.detected_reason,event_id=CASE WHEN ufc_library_files.manual THEN ufc_library_files.event_id ELSE excluded.event_id END,component=CASE WHEN ufc_library_files.manual THEN ufc_library_files.component ELSE excluded.component END,resolution=CASE WHEN ufc_library_files.manual THEN ufc_library_files.resolution ELSE excluded.resolution END,group_key=CASE WHEN ufc_library_files.manual THEN ufc_library_files.group_key ELSE excluded.group_key END,state=CASE WHEN ufc_library_files.manual THEN CASE WHEN ufc_library_files.state<>'ignored' AND (ufc_library_files.state='review' OR ufc_library_files.size<>excluded.size OR ufc_library_files.modified<>excluded.modified) THEN 'review' ELSE ufc_library_files.state END ELSE excluded.state END,reason=CASE WHEN ufc_library_files.manual THEN CASE WHEN ufc_library_files.state<>'ignored' AND (ufc_library_files.state='review' OR ufc_library_files.size<>excluded.size OR ufc_library_files.modified<>excluded.modified) THEN 'File changed since manual confirmation; review required' ELSE ufc_library_files.reason END ELSE excluded.reason END`, f.ID, root.ID, f.Path, f.Size, f.Modified, f.EventID, f.Component, f.Resolution, f.State, f.Reason, f.Group, run)
			if err != nil {
				return err
			}
		}
		if complete {
			_, err = tx.ExecContext(ctx, "UPDATE ufc_library_files SET present=FALSE,revision=revision+1 WHERE root_id=$1 AND last_seen<>$2 AND present=TRUE", root.ID, run)
			if err != nil {
				return err
			}
		}
		state, message := "available", ""
		if !complete {
			state = "unavailable"
			message = "Location could not be fully read; saved inventory retained"
		}
		_, err = tx.ExecContext(ctx, "UPDATE ufc_library_roots SET state=$1,message=$2 WHERE id=$3", state, message, root.ID)
		return err
	})
}

const libraryFileColumns = "f.id,f.revision,f.root_id,r.path,f.relative_path,f.size,f.modified,f.event_id,f.component,f.resolution,CASE WHEN f.event_id<>'' AND f.state<>'ignored' AND NOT EXISTS(SELECT 1 FROM ufc_events e WHERE e.id=f.event_id AND e.manual_corrections_json NOT LIKE '%\"deleted\":true%') THEN 'review' ELSE f.state END,CASE WHEN f.event_id<>'' AND f.state<>'ignored' AND NOT EXISTS(SELECT 1 FROM ufc_events e WHERE e.id=f.event_id AND e.manual_corrections_json NOT LIKE '%\"deleted\":true%') THEN 'Assigned event was removed from the catalogue; rescan, then use detected values or confirm a new event' ELSE f.reason END,f.manual,f.present,f.group_key,f.detected_event,f.detected_component,f.detected_resolution,f.detected_state,f.detected_reason,r.enabled,r.state"

func decodeLibraryFile(row scanner) (library.File, error) {
	var f library.File
	var enabled bool
	var state string
	err := row.Scan(&f.ID, &f.Revision, &f.RootID, &f.RootPath, &f.Path, &f.Size, &f.Modified, &f.EventID, &f.Component, &f.Resolution, &f.State, &f.Reason, &f.Manual, &f.Present, &f.Group, &f.DetectedEvent, &f.DetectedComponent, &f.DetectedResolution, &f.DetectedState, &f.DetectedReason, &enabled, &state)
	f.Available = f.Present && enabled && state == "available"
	return f, err
}

// LibraryFile reads a saved observation without accessing its media path.
func (r *Repository) LibraryFile(ctx context.Context, id string) (library.File, error) {
	return decodeLibraryFile(r.db.QueryRowContext(ctx, "SELECT "+libraryFileColumns+" FROM ufc_library_files f JOIN ufc_library_roots r ON r.id=f.root_id WHERE f.id=$1", id))
}

// LibraryFiles pages expanded rows for one compact group.
func (r *Repository) LibraryFiles(ctx context.Context, group string, offset int) (*library.FilePage, error) {
	p := &library.FilePage{Files: []library.File{}}
	err := r.readTransaction(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_library_files WHERE group_key=$1", group).Scan(&p.Total); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, "SELECT "+libraryFileColumns+" FROM ufc_library_files f JOIN ufc_library_roots r ON r.id=f.root_id WHERE f.group_key=$1 ORDER BY f.relative_path,f.root_id LIMIT 50 OFFSET $2", group, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			f, err := decodeLibraryFile(rows)
			if err != nil {
				return err
			}
			p.Files = append(p.Files, f)
		}
		return rows.Err()
	})
	return p, err
}

// EditLibraryFile saves protected identification at an expected observation revision.
func (r *Repository) EditLibraryFile(ctx context.Context, id string, edit library.FileEdit, actor string) (library.File, error) {
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE ufc_library_settings SET revision=revision+1 WHERE id=1")
		if err != nil {
			return err
		}
		f, err := decodeLibraryFile(tx.QueryRowContext(ctx, "SELECT "+libraryFileColumns+" FROM ufc_library_files f JOIN ufc_library_roots r ON r.id=f.root_id WHERE f.id=$1", id))
		if err != nil {
			return err
		}
		if f.Revision != edit.Revision {
			return catalogue.ErrRevisionConflict
		}
		manual := true
		switch edit.Mode {
		case "auto":
			f.EventID = f.DetectedEvent
			f.Component = f.DetectedComponent
			f.Resolution = f.DetectedResolution
			f.State = f.DetectedState
			f.Reason = f.DetectedReason
			manual = false
		case "ignore":
			f.State = "ignored"
			f.Reason = "Ignored by manual review"
		case "confirm":
			if !f.Available {
				return errors.Wrap(catalogue.ErrInvalidEdit, "rescan an available location before confirming presence")
			}
			var event string
			err = tx.QueryRowContext(ctx, `SELECT id FROM ufc_events WHERE id=$1 AND manual_corrections_json NOT LIKE '%"deleted":true%'`, edit.EventID).Scan(&event)
			if err != nil {
				return errors.Wrap(catalogue.ErrInvalidEdit, "choose an existing event")
			}
			f.EventID = event
			f.Component = edit.Component
			f.Resolution = edit.Resolution
			f.State = "confirmed"
			f.Reason = "Protected manual identification"
		default:
			return errors.Wrap(catalogue.ErrInvalidEdit, "invalid library review operation")
		}
		group := "folder:" + library.Key(f.RootID+"\x00"+path.Dir(f.Path))
		if f.EventID != "" {
			group = "event:" + f.EventID
		}
		result, err := tx.ExecContext(ctx, "UPDATE ufc_library_files SET revision=revision+1,event_id=$1,component=$2,resolution=$3,state=$4,reason=$5,manual=$6,group_key=$7,actor=$8 WHERE id=$9 AND revision=$10", f.EventID, f.Component, f.Resolution, f.State, f.Reason, manual, group, actor, id, edit.Revision)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return catalogue.ErrRevisionConflict
		}
		return nil
	})
	if err != nil {
		return library.File{}, err
	}
	return r.LibraryFile(ctx, id)
}

// AdoptLibraryEvents adds only newly resolved source identities, never overwriting records.
func (r *Repository) AdoptLibraryEvents(ctx context.Context, snapshot *catalogue.Snapshot) error {
	if snapshot == nil {
		return errors.New("missing library metadata")
	}
	return r.transaction(ctx, func(tx *sql.Tx) error {
		for _, facts := range snapshot.Facts {
			if err := catalogue.ValidateFacts(facts); err != nil {
				return err
			}
			var n int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_events WHERE source_page_id=$1", facts.PageID).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				continue
			}
			id, err := catalogueIdentity(ctx, tx, facts.PageID)
			if err != nil {
				return err
			}
			e, err := catalogue.Merge(catalogue.Event{ID: id}, facts)
			if err != nil {
				return err
			}
			for i, a := range e.Aliases {
				if a.Namespace == "title" || !a.Current || !a.Verified {
					continue
				}
				if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_event_aliases WHERE namespace=$1 AND normalized_value=$2 AND is_current=TRUE AND verified=TRUE", a.Namespace, catalogue.Normalize(a.Value)).Scan(&n); err != nil {
					return err
				}
				if n > 0 {
					e.Aliases[i].Verified = false
					e.Review = appendReview(e.Review, "identifier_owned_by_existing_event")
				}
			}
			if err := saveEvent(ctx, tx, e, 0); err != nil {
				return err
			}
		}
		return nil
	})
}

// LibraryView builds compact coverage summaries without returning all file rows.
func (r *Repository) LibraryView(ctx context.Context) (*library.View, error) {
	view := &library.View{Groups: []library.Group{}}
	err := r.readTransaction(ctx, func(tx *sql.Tx) error {
		var err error
		view.Wanted, err = libraryWanted(ctx, tx)
		if err != nil {
			return err
		}
		view.Settings, err = librarySettings(ctx, tx)
		if err != nil {
			return err
		}
		unknown := false
		for _, root := range view.Settings.Roots {
			if root.Enabled {
				view.Configured = true
				if root.State != "available" {
					unknown = true
				}
			}
		}
		rows, err := tx.QueryContext(ctx, `SELECT f.group_key,f.event_id,COALESCE(e.name,''),e.event_date,f.component,f.state,f.present,r.id,r.enabled,r.state,COUNT(*),MIN(f.relative_path),r.path FROM ufc_library_files f JOIN ufc_library_roots r ON r.id=f.root_id LEFT JOIN ufc_events e ON e.id=f.event_id GROUP BY f.group_key,f.event_id,e.name,e.event_date,f.component,f.state,f.present,r.id,r.enabled,r.state,r.path`)
		if err != nil {
			return err
		}
		defer rows.Close()
		groups := map[string]*library.Group{}
		locations := map[string]map[string]bool{}
		for rows.Next() {
			var key, event, name, component, state, root, rootState, relative, rootPath string
			var date *string
			var present, enabled bool
			var count int
			if err := rows.Scan(&key, &event, &name, &date, &component, &state, &present, &root, &enabled, &rootState, &count, &relative, &rootPath); err != nil {
				return err
			}
			g := groups[key]
			if event != "" && name == "" && state != "ignored" {
				state = "review"
			}
			if g == nil {
				if name == "" {
					name = "Unidentified: " + unidentifiedLibraryFolder(relative, rootPath)
				}
				g = &library.Group{ID: key, EventID: event, Name: name, Date: date, Components: map[string]string{}}
				for _, c := range []string{"main_card", "prelims", "early_prelims", "countdown", "embedded", "weigh_ins"} {
					g.Components[c] = "missing"
					if unknown {
						g.Components[c] = "unavailable"
					}
				}
				groups[key] = g
				locations[key] = map[string]bool{}
			}
			g.Files += count
			locations[key][root] = true
			available := present && enabled && rootState == "available"
			if !available {
				g.Unavailable += count
			}
			if state == "extra" || state == "ignored" {
				g.Extras += count
				continue
			}
			if state == "review" {
				g.Review += count
				if component == "unknown" && available {
					for c, current := range g.Components {
						if current != "present" {
							g.Components[c] = "review"
						}
					}
				}
			}
			if _, ok := g.Components[component]; !ok {
				continue
			}
			current := g.Components[component]
			if available && state == "confirmed" {
				g.Components[component] = "present"
			} else if current != "present" && available && state == "review" {
				g.Components[component] = "review"
			} else if current != "present" && current != "review" && !available && (rootState != "available" || !enabled) {
				g.Components[component] = "unavailable"
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for key, g := range groups {
			g.Wanted = view.Wanted[g.EventID]
			g.Locations = len(locations[key])
			view.Groups = append(view.Groups, *g)
		}
		if len(view.Groups) > 0 {
			view.Configured = true
		}
		return nil
	})
	sort.Slice(view.Groups, func(i, j int) bool {
		a, b := view.Groups[i], view.Groups[j]
		if (a.EventID != "") != (b.EventID != "") {
			return a.EventID != ""
		}
		if a.Date != nil && b.Date != nil && *a.Date != *b.Date {
			return *a.Date > *b.Date
		}
		if a.Date != nil && b.Date == nil {
			return true
		}
		if a.Date == nil && b.Date != nil {
			return false
		}
		return a.Name < b.Name
	})
	return view, err
}

func unidentifiedLibraryFolder(relative, root string) string {
	for folder := path.Dir(relative); folder != "." && folder != "/"; folder = path.Dir(folder) {
		name := path.Base(folder)
		switch strings.ToLower(name) {
		case "featurettes", "extras", "specials", "bonus":
			continue
		}
		return name
	}
	return filepath.Base(root)
}

func guardLibraryCoverage(ctx context.Context, tx *sql.Tx, driver, event, component string) error {
	lock := ""
	if driver == "postgres" {
		lock = " FOR SHARE"
	}
	var running bool
	if err := tx.QueryRowContext(ctx, "SELECT running FROM ufc_library_settings WHERE id=1"+lock).Scan(&running); err != nil {
		return err
	}
	if running {
		return errors.Wrap(catalogue.ErrInvalidEdit, "library scan is running; wait before submitting another copy")
	}
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_library_roots WHERE enabled=TRUE AND state<>'available'").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return errors.Wrap(catalogue.ErrInvalidEdit, "library locations are unscanned or unavailable; review Library before downloading")
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ufc_library_files f JOIN ufc_library_roots r ON r.id=f.root_id WHERE f.event_id=$1 AND (f.component=$2 OR f.component='unknown') AND f.state IN ('confirmed','review') AND (f.present=TRUE OR r.state<>'available' OR r.enabled=FALSE)`, event, component).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return errors.Wrap(catalogue.ErrInvalidEdit, "library already contains this component or needs review; check Library before downloading another copy")
	}
	return nil
}
