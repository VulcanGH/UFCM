// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type dueRepository struct {
	serviceRepository
	due atomic.Bool
}

func (r *dueRepository) ClaimCatalogueRefresh(ctx context.Context, at time.Time, scheduled bool) (bool, error) {
	if scheduled && !r.due.Swap(false) {
		return false, nil
	}
	return true, r.RecordCatalogueAttempt(ctx, at)
}

func TestPeriodicWakeCoalescesAndStops(t *testing.T) {
	repo := &dueRepository{}
	repo.due.Store(true)
	entered := make(chan struct{})
	s := New(repo, serviceSource{read: func(ctx context.Context) (*Snapshot, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}}, zerolog.Nop())
	require.NoError(t, s.Start(t.Context()))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("due refresh did not start")
	}
	accepted, err := s.Refresh(t.Context())
	require.NoError(t, err)
	require.True(t, accepted.Coalesced)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	require.EqualValues(t, 1, repo.attempts.Load())
	require.EqualValues(t, 1, repo.failures.Load())
}
