// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/autobrr/autobrr/pkg/dbsnapshot"

	"github.com/Masterminds/semver/v3"
	_ "modernc.org/sqlite"
)

// Seed binds installer-owned baseline files without making them an unsigned update channel.
type Seed struct {
	Version string `json:"version"`
	Schema  string `json:"schema"`
	Files   []File `json:"files"`
}

// Bootstrap persists selection before activation so a lost acknowledgement never rewinds data.
type Bootstrap struct {
	Active     string `json:"active"`
	Previous   string `json:"previous"`
	Candidate  string `json:"candidate"`
	Phase      string `json:"phase"`
	Database   string `json:"database"`
	Config     string `json:"config"`
	Schema     string `json:"schema"`
	Highwater  int64  `json:"highwater"`
	FailedID   string `json:"failed_id"`
	DeferredID string `json:"deferred_id"`
	Backup     string `json:"backup"`
}

// Launcher supervises exactly one managed profile and never replaces its own executable.
type Launcher struct {
	Root           string
	Install        string
	Config         string
	Executable     string
	Trust          Trust
	StartupTimeout time.Duration
	Command        func(string, ...string) *exec.Cmd
}

func (l *Launcher) runtime(id string) (string, string, error) {
	if id == "seed" {
		dir := filepath.Join(l.Install, "seed")
		var seed Seed
		if err := ReadJSON(filepath.Join(dir, "seed.json"), &seed); err != nil {
			return "", "", err
		}
		if seed.Version == "" || len(seed.Files) < 2 {
			return "", "", fmt.Errorf("invalid installer baseline")
		}
		for _, f := range seed.Files {
			if !allowedFile(f.Name) || !digest(f.SHA256) {
				return "", "", fmt.Errorf("invalid baseline file")
			}
			if err := verifyFile(dir, f); err != nil {
				return "", "", err
			}
		}
		return dir, seed.Version, nil
	}
	if !digest(id) {
		return "", "", fmt.Errorf("invalid selected runtime")
	}
	dir := filepath.Join(l.Root, "versions", id)
	var e Envelope
	if err := ReadJSON(filepath.Join(dir, "release.json"), &e); err != nil {
		return "", "", err
	}
	if ID(e) != id {
		return "", "", fmt.Errorf("runtime identity mismatch")
	}
	if err := NewClient(l.Trust).VerifyRuntime(dir, e, 0); err != nil {
		return "", "", err
	}
	m, err := l.Trust.Verify(e, time.Now(), 0, false)
	return dir, m.Version, err
}

func verifyFile(dir string, f File) error {
	if err := SafeDirectory(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, f.Name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || reparse(info) || info.Size() != f.Size {
		return fmt.Errorf("invalid baseline runtime file")
	}
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	h := sha256.New()
	if _, err = io.Copy(h, in); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
		return fmt.Errorf("baseline runtime hash mismatch")
	}
	return nil
}

// Run keeps the old version selected until a new, authenticated process is ready to activate.
func (l *Launcher) Run(ctx context.Context) error {
	if err := SafeDirectory(l.Root); err != nil {
		return err
	}
	if err := os.MkdirAll(l.Root, 0700); err != nil {
		return err
	}
	if err := PrivateDirectory(l.Root); err != nil {
		return err
	}
	unlock, err := Lock(l.Root)
	if err != nil {
		return err
	}
	defer unlock()
	if err := CheckMaintenance(l.Root); err != nil {
		return err
	}
	if err := SafeDirectory(l.Config); err != nil {
		return err
	}
	if err := os.MkdirAll(l.Config, 0700); err != nil {
		return err
	}
	profileUnlock, err := Lock(l.Config)
	if err != nil {
		return err
	}
	defer profileUnlock()
	if err := CheckMaintenance(l.Config); err != nil {
		return err
	}
	journal := filepath.Join(l.Root, "bootstrap-state.json")
	state := Bootstrap{Active: "seed", Phase: "active"}
	if err = ReadJSON(journal, &state); err != nil && !os.IsNotExist(err) {
		return err
	}
	if state.Config != "" && !samePath(state.Config, l.Config) {
		return fmt.Errorf("this installation is bound to another configuration")
	}
	if state.Phase == "trial" {
		schema, err := SchemaFingerprint(ctx, state.Database)
		if err != nil || schema != state.Schema {
			return fmt.Errorf("update recovery required: schema changed during interrupted trial; database retained")
		}
		state.FailedID = state.Candidate
		state.Candidate = ""
		state.Phase = "active"
		if err = WriteJSON(journal, state); err != nil {
			return err
		}
	}
	for {
		dir, version, err := l.runtime(state.Active)
		if err != nil {
			return err
		}
		host, err := l.runChild(ctx, dir, version, func(h Host) error {
			if !samePath(h.Config, l.Config) {
				return fmt.Errorf("runtime opened a different configuration")
			}
			if state.Config != "" && (!samePath(state.Config, h.Config) || !samePath(state.Database, h.Database)) {
				return fmt.Errorf("runtime database identity changed")
			}
			state.Config = h.Config
			state.Database = h.Database
			return WriteJSON(journal, state)
		})
		if err != nil {
			return err
		}
	processRequest:
		if err := l.Prune(state); err != nil {
			return err
		}
		var request Request
		requestPath := filepath.Join(l.Root, "request.json")
		if err = ReadJSON(requestPath, &request); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
		if host.Driver != "sqlite" || !samePath(request.Config, host.Config) || !samePath(request.Database, host.Database) {
			return fmt.Errorf("update request is not bound to a supported SQLite profile")
		}
		m, err := l.Trust.Verify(request.Envelope, time.Now(), state.Highwater, true)
		if err != nil {
			return err
		}
		id := ID(request.Envelope)
		if m.Sequence <= state.Highwater {
			return fmt.Errorf("update sequence was already consumed")
		}
		if id == state.FailedID || id == state.Active {
			return fmt.Errorf("update was already applied or failed")
		}
		installed, installedErr := semver.NewVersion(version)
		candidate, candidateErr := semver.NewVersion(m.Version)
		if installedErr != nil || candidateErr != nil {
			return fmt.Errorf("cannot compare update versions")
		}
		if !candidate.GreaterThan(installed) {
			return os.Rename(requestPath, filepath.Join(l.Root, "obsolete-request-"+id+".json"))
		}
		next, version, err := l.runtime(id)
		if err != nil {
			return err
		}
		state.Schema, err = SchemaFingerprint(ctx, host.Database)
		if err != nil {
			return err
		}
		backupDir := filepath.Join(l.Root, "backups", id)
		backupErr := func() error {
			if err := os.MkdirAll(backupDir, 0700); err != nil {
				return err
			}
			attempt, err := os.MkdirTemp(backupDir, "attempt-")
			if err != nil {
				return err
			}
			backupDir = attempt
			if err := dbsnapshot.SQLite(ctx, host.Database, filepath.Join(backupDir, "database.db")); err != nil {
				return err
			}
			return copyConfig(host.Config, filepath.Join(backupDir, "config.toml"))
		}()
		if backupErr != nil {
			state.DeferredID = id
			if err = WriteJSON(journal, state); err != nil {
				return err
			}
			if err = os.Remove(requestPath); err != nil {
				return err
			}
			continue
		}
		state.DeferredID = ""
		state.Backup = filepath.ToSlash(filepath.Join(id, filepath.Base(backupDir)))
		state.Candidate = id
		state.Phase = "trial"
		state.Highwater = m.Sequence
		if err = WriteJSON(journal, state); err != nil {
			return err
		}
		if err = os.Remove(requestPath); err != nil {
			return err
		}
		activated := false
		previousActive := state.Active
		trialHost, trialErr := l.runChild(ctx, next, version, func(h Host) error {
			if h.Driver != "sqlite" || !samePath(h.Database, state.Database) || !samePath(h.Config, state.Config) {
				return fmt.Errorf("trial changed database identity")
			}
			nextState := state
			nextState.Previous = state.Active
			nextState.Active = id
			nextState.Candidate = ""
			nextState.Phase = "active"
			if err := WriteJSON(journal, nextState); err != nil {
				return err
			}
			state = nextState
			activated = true
			return nil
		})
		if trialErr != nil {
			if activated {
				return fmt.Errorf("activated runtime stopped; database retained: %w", trialErr)
			}
			if _, running := trialErr.(*runningChildError); running {
				return trialErr
			}
			schema, schemaErr := SchemaFingerprint(ctx, state.Database)
			if schemaErr != nil || schema != state.Schema {
				return fmt.Errorf("update recovery required: trial changed schema; database retained")
			}
			state.FailedID = id
			state.Active = previousActive
			state.Candidate = ""
			state.Phase = "active"
			if err = WriteJSON(journal, state); err != nil {
				return err
			}
			continue
		}
		// A cleanly stopped activated runtime may have queued the next update.
		if err := l.Prune(state); err != nil {
			return err
		}
		if _, err = os.Lstat(requestPath); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
		host = trialHost
		goto processRequest
	}
}

func (l *Launcher) runChild(ctx context.Context, dir, version string, activate func(Host) error) (Host, error) {
	var host Host
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return host, err
	}
	defer listener.Close()
	timeout := l.StartupTimeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(timeout))
	nonce := rand.Text() + rand.Text()
	command := l.Command
	if command == nil {
		command = exec.Command
	}
	child := command(filepath.Join(dir, "ufc-manager-desktop.exe"), "--config", l.Config)
	workingDirectory, err := os.Getwd()
	if err != nil {
		return host, err
	}
	child.Dir = workingDirectory
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		switch strings.ToUpper(key) {
		case "UFC_MANAGER_RUNTIME_ROOT", "UFC_MANAGER_LAUNCHER", "UFC_MANAGER_BOOTSTRAP_ADDR", "UFC_MANAGER_BOOTSTRAP_NONCE":
			continue
		}
		child.Env = append(child.Env, v)
	}
	child.Env = append(child.Env, "UFC_MANAGER_RUNTIME_ROOT="+l.Root, "UFC_MANAGER_LAUNCHER="+l.Executable, "UFC_MANAGER_BOOTSTRAP_ADDR="+listener.Addr().String(), "UFC_MANAGER_BOOTSTRAP_NONCE="+nonce)
	if err = child.Start(); err != nil {
		return host, err
	}
	exited := make(chan error, 1)
	go func() { exited <- child.Wait(); _ = listener.Close() }()
	conn, err := listener.Accept()
	if err != nil {
		select {
		case exitErr := <-exited:
			return host, fmt.Errorf("runtime exited before readiness: %v", exitErr)
		default:
			return host, &runningChildError{cause: fmt.Errorf("runtime did not become ready; it was not killed or replaced")}
		}
	}
	defer conn.Close()
	failure := func(cause error) (Host, error) {
		_ = conn.Close()
		select {
		case <-exited:
			return host, cause
		case <-time.After(5 * time.Second):
			return host, &runningChildError{cause: cause}
		}
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if err = json.NewDecoder(io.LimitReader(conn, 64<<10)).Decode(&host); err != nil {
		return failure(err)
	}
	if host.Nonce != nonce || host.PID != child.Process.Pid || strings.TrimPrefix(host.Version, "v") != strings.TrimPrefix(version, "v") {
		return failure(fmt.Errorf("runtime health identity mismatch"))
	}
	expectedSchema := ""
	if filepath.Dir(dir) == l.Install {
		var seed Seed
		if err = ReadJSON(filepath.Join(dir, "seed.json"), &seed); err != nil {
			return failure(err)
		}
		expectedSchema = seed.Schema
	} else {
		var envelope Envelope
		if err = ReadJSON(filepath.Join(dir, "release.json"), &envelope); err != nil {
			return failure(err)
		}
		manifest, verifyErr := l.Trust.Verify(envelope, time.Now(), 0, false)
		if verifyErr != nil {
			return failure(verifyErr)
		}
		expectedSchema = manifest.Schema
	}
	if expectedSchema != "" && host.Schema != expectedSchema {
		return failure(fmt.Errorf("runtime migration sources do not match release"))
	}
	if err = activate(host); err != nil {
		return failure(err)
	}
	if err = json.NewEncoder(conn).Encode(struct {
		Nonce    string `json:"nonce"`
		Activate bool   `json:"activate"`
	}{nonce, true}); err != nil {
		return failure(err)
	}
	_ = conn.Close()
	select {
	case err = <-exited:
		return host, err
	case <-ctx.Done():
		return host, fmt.Errorf("launcher interrupted; runtime retained: %w", ctx.Err())
	}
}

type runningChildError struct{ cause error }

func (e *runningChildError) Error() string {
	return "Update recovery paused while the trial process may still be running: " + e.cause.Error()
}

func samePath(a, b string) bool {
	x, e1 := filepath.Abs(a)
	y, e2 := filepath.Abs(b)
	return e1 == nil && e2 == nil && strings.EqualFold(filepath.Clean(x), filepath.Clean(y))
}

func copyConfig(config, destination string) error {
	source := filepath.Join(config, "config.toml")
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || reparse(info) || info.Size() > 4<<20 {
		return fmt.Errorf("unsafe config backup source")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := out.Write(data)
	syncErr := out.Sync()
	closeErr := out.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

// SchemaFingerprint permits fallback only when both schema and migration histories are unchanged.
func SchemaFingerprint(ctx context.Context, path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || reparse(info) {
		return "", fmt.Errorf("invalid SQLite recovery database")
	}
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(filepath.ToSlash(path))+"?mode=ro")
	if err != nil {
		return "", err
	}
	defer db.Close()
	h := sha256.New()
	for _, query := range []string{`SELECT type,name,tbl_name,COALESCE(sql,'') FROM sqlite_master ORDER BY type,name`, `SELECT id,version FROM schema_migrations ORDER BY id`, `SELECT id,version FROM ufc_schema_migrations ORDER BY id`} {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return "", err
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		for rows.Next() {
			values := make([]any, len(cols))
			targets := make([]any, len(cols))
			for i := range values {
				targets[i] = &values[i]
			}
			if err = rows.Scan(targets...); err != nil {
				rows.Close()
				return "", err
			}
			data, err := json.Marshal(values)
			if err != nil {
				rows.Close()
				return "", err
			}
			h.Write(data)
			h.Write([]byte{0})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
