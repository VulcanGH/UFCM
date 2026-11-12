// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateMatchingKeysWithoutReplacingExistingPair(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "signing")
	require.NoError(t, generate(directory))
	read := func(name string) []byte {
		raw, err := os.ReadFile(filepath.Join(directory, name))
		require.NoError(t, err)
		decoded, err := base64.StdEncoding.DecodeString(string(raw))
		require.NoError(t, err)
		return decoded
	}
	private, public := read("private-key.txt"), read("public-key.txt")
	require.Len(t, private, ed25519.PrivateKeySize)
	require.Len(t, public, ed25519.PublicKeySize)
	message := []byte("isolated signing fixture")
	require.True(t, ed25519.Verify(public, message, ed25519.Sign(private, message)))
	require.Error(t, generate(directory))
	require.Equal(t, private, read("private-key.txt"))
	require.Equal(t, public, read("public-key.txt"))
}
