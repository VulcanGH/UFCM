// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"fmt"
	"testing"
	"time"

	"github.com/autobrr/go-cache/ttlcache"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestOIDCEvictionUsesInsertionOrder(t *testing.T) {
	cache := ttlcache.New[string, pendingOIDCLogin](ttlcache.DisableUpdateTime(true))
	t.Cleanup(cache.Close)
	cache.Set("first", pendingOIDCLogin{sequence: 1}, time.Hour)
	for i := 2; i <= oidcMaxPendingLogins; i++ {
		cache.Set(fmt.Sprint(i), pendingOIDCLogin{sequence: uint64(i)}, time.Minute)
	}
	handler := &OIDCHandler{pendingLogins: cache, log: zerolog.Nop()}
	handler.m.Lock()
	handler.evictOldestLoginIfFull()
	handler.m.Unlock()
	_, kept := cache.Get("first")
	require.False(t, kept)
	require.Len(t, cache.GetKeys(), oidcMaxPendingLogins-1)
}
