// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package maintenance

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFreezeWaitsForEveryLeaseAndExcludesNewWork(t *testing.T) {
	var gate Gate
	a, ok := gate.Enter()
	require.True(t, ok)
	b, ok := gate.Enter()
	require.True(t, ok)
	require.False(t, gate.Freeze())
	a()
	a()
	require.False(t, gate.Freeze())
	b()
	require.True(t, gate.Freeze())
	_, ok = gate.Enter()
	require.False(t, ok)
	gate.Resume()
	c, ok := gate.Enter()
	require.True(t, ok)
	c()
}

func TestDrainAllowsAdmittedNestedWorkButPausesNewOperations(t *testing.T) {
	var gate Gate
	ctx, parent, ok := gate.EnterContext(context.Background())
	require.True(t, ok)
	gate.Drain()
	_, ok = gate.Enter()
	require.False(t, ok)
	_, nested, ok := gate.EnterContext(ctx)
	require.True(t, ok)
	parent()
	_, _, ok = gate.EnterContext(ctx)
	require.False(t, ok, "an expired parent cannot admit new work")
	require.False(t, gate.Freeze())
	nested()
	require.True(t, gate.Freeze())
	gate.Resume()
	_, last, ok := gate.EnterContext(context.Background())
	require.True(t, ok)
	last()
}
