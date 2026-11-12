// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Host describes the managed process without relying on an HTTP port as proof of readiness.
type Host struct {
	Nonce    string `json:"nonce"`
	PID      int    `json:"pid"`
	Version  string `json:"version"`
	Driver   string `json:"driver"`
	Database string `json:"database"`
	Config   string `json:"config"`
	Schema   string `json:"schema"`
}

// ManagedRoot is provided only by the stable launcher, never by the browser.
func ManagedRoot() string { return os.Getenv("UFC_MANAGER_RUNTIME_ROOT") }

// HostReady blocks before workers start until the owning launcher commits activation.
func HostReady(ctx context.Context, h Host) error {
	endpoint := os.Getenv("UFC_MANAGER_BOOTSTRAP_ADDR")
	if endpoint == "" {
		return nil
	}
	address, err := net.ResolveTCPAddr("tcp", endpoint)
	if err != nil || !address.IP.IsLoopback() {
		return fmt.Errorf("invalid launcher endpoint")
	}
	h.Nonce = os.Getenv("UFC_MANAGER_BOOTSTRAP_NONCE")
	h.Schema = SchemaDigest
	h.PID = os.Getpid()
	if len(h.Nonce) < 32 {
		return fmt.Errorf("invalid launcher nonce")
	}
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	if err = json.NewEncoder(conn).Encode(h); err != nil {
		return err
	}
	var reply struct {
		Nonce    string `json:"nonce"`
		Activate bool   `json:"activate"`
	}
	if err = json.NewDecoder(conn).Decode(&reply); err != nil {
		return err
	}
	if !reply.Activate || reply.Nonce != h.Nonce {
		return fmt.Errorf("launcher did not activate this runtime")
	}
	return nil
}

// ReadJSON bounds local bootstrap input and rejects linked files.
func ReadJSON(path string, out any) error {
	if err := SafeDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || reparse(info) || info.Size() > ManifestLimit*3 {
		return fmt.Errorf("unsafe bootstrap journal")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// Request binds a pending handover to the current configuration and signed runtime.
type Request struct {
	Envelope Envelope `json:"envelope"`
	Database string   `json:"database"`
	Config   string   `json:"config"`
}
