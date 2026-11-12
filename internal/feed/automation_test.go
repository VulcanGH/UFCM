// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package feed

import (
	"context"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestDurableObserverBeforeCacheAcknowledgement(t *testing.T) {
	cache := &recordingCacheRepo{}
	j := &refreshJob{log: zerolog.Nop(), feed: &domain.Feed{ID: 1}, src: &stubSource{res: &fetchResult{entries: []entry{newEntry("UFC.999.1080p", time.Now())}}}, repo: &recordingFeedRepo{}, cacheRepo: cache}
	j.observePage = func(context.Context, *domain.Feed, []SearchItem) error { return errors.New("database unavailable") }
	_, err := j.fetchNew(t.Context())
	require.Error(t, err)
	require.Empty(t, cache.stored)
	seen := 0
	j.observePage = func(_ context.Context, _ *domain.Feed, items []SearchItem) error { seen += len(items); return nil }
	_, err = j.fetchNew(t.Context())
	require.NoError(t, err)
	require.Len(t, cache.stored, 1)
	cache.existing = map[string]bool{"UFC.999.1080p": true}
	_, err = j.fetchNew(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, seen, "cached feed pages remain observable after automation is enabled")
}
