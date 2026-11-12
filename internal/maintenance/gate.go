// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

// Package maintenance coordinates updater handover with in-flight application work.
package maintenance

import (
	"context"
	"sync"
)

// Gate permits existing work to finish before atomically excluding new work.
type Gate struct {
	mu       sync.Mutex
	active   int
	frozen   bool
	draining bool
}

type contextKey struct{ gate *Gate }
type lease struct{ active bool }

// Application is shared by API and background entry points in one application process.
var Application Gate

// Enter returns a release function, or false when maintenance excludes new work.
func (g *Gate) Enter() (func(), bool) {
	_, release, allowed := g.EnterContext(context.Background())
	return release, allowed
}

// EnterContext permits an admitted operation to finish nested work while new operations drain.
func (g *Gate) EnterContext(ctx context.Context) (context.Context, func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	parent, _ := ctx.Value(contextKey{g}).(*lease)
	if g.frozen || (g.draining && (parent == nil || !parent.active)) {
		return ctx, func() {}, false
	}
	g.active++
	current := &lease{active: true}
	var once sync.Once
	return context.WithValue(ctx, contextKey{g}, current), func() { once.Do(func() { g.mu.Lock(); current.active = false; g.active--; g.mu.Unlock() }) }, true
}

// Drain pauses new operations without cancelling admitted work.
func (g *Gate) Drain() { g.mu.Lock(); g.draining = true; g.mu.Unlock() }

// Freeze succeeds only when every tracked operation has finished.
func (g *Gate) Freeze() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.frozen || g.active != 0 {
		return false
	}
	g.frozen = true
	g.draining = false
	return true
}

// Resume abandons a handover that has not stopped the application.
func (g *Gate) Resume() { g.mu.Lock(); g.frozen = false; g.draining = false; g.mu.Unlock() }
