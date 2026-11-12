// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/updater"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var version, commit, dir, payload, output string
	var sequence int64
	var seed, printSchema bool
	flag.StringVar(&version, "version", "", "exact stable release tag")
	flag.StringVar(&commit, "commit", "", "source commit")
	flag.StringVar(&dir, "runtime", "", "runtime directory")
	flag.StringVar(&payload, "payload", "", "runtime ZIP")
	flag.StringVar(&output, "output", "", "manifest output directory")
	flag.Int64Var(&sequence, "sequence", 0, "monotonically increasing release sequence")
	flag.BoolVar(&seed, "seed", false, "write installer baseline metadata without signing")
	flag.BoolVar(&printSchema, "print-schema", false, "print the migration source digest")
	flag.Parse()
	schema, err := updater.SourceSchemaDigest(".")
	if err != nil {
		return err
	}
	if printSchema {
		fmt.Println(schema)
		return nil
	}
	var files []updater.File
	for _, name := range []string{"ufc-manager-desktop.exe", "ufc-manager.exe", "LICENSE.txt", "README.txt"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		files = append(files, updater.File{Name: name, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])})
	}
	if seed {
		return updater.WriteJSON(filepath.Join(dir, "seed.json"), updater.Seed{Version: version, Schema: schema, Files: files})
	}
	rawKey, err := base64.StdEncoding.DecodeString(os.Getenv("UFC_UPDATE_PRIVATE_KEY"))
	if err != nil || len(rawKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("a valid release signing key must be provided through the protected environment")
	}
	key := ed25519.PrivateKey(rawKey)
	trust, err := updater.ProductionTrust()
	if err != nil {
		return err
	}
	if !key.Public().(ed25519.PublicKey).Equal(trust.Key) {
		return fmt.Errorf("release key does not match the pinned public key")
	}
	data, err := os.ReadFile(payload)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	now := time.Now().UTC()
	m := updater.Manifest{Format: 1, Product: "ufc-manager", Channel: "stable", Version: version, Sequence: sequence, Commit: commit, Issued: now, Expires: now.AddDate(1, 0, 0), OS: "windows", Arch: "amd64", MinLauncher: updater.Protocol, URL: "https://github.com/" + updater.Repository + "/releases/download/" + version + "/" + filepath.Base(payload), Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Files: files}
	m.Schema = schema
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	signature := ed25519.Sign(key, raw)
	if _, err = trust.Verify(updater.Envelope{Manifest: raw, Signature: signature}, now, 0, true); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(output, "ufc-update.json"), raw, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(output, "ufc-update.sig"), signature, 0644)
}
