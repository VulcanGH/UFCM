// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build integration

package database

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestCatalogueEventListBothEngines(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := uint32(listener.Addr().(*net.TCPAddr).Port)
	require.NoError(t, listener.Close())
	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(port).Version(embeddedpostgres.V17).Locale("C").Encoding("UTF8").Username("postgres").Password("events-test-only").Database("events").CachePath(filepath.Join(cache, "ufc-foundation", "archive")).BinariesPath(filepath.Join(cache, "ufc-foundation", "binaries")).RuntimePath(filepath.Join(t.TempDir(), "runtime")))
	require.NoError(t, pg.Start())
	t.Cleanup(func() { require.NoError(t, pg.Stop()) })
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			dsn := filepath.Join(t.TempDir(), "events.db")
			if engine == "postgres" {
				dsn = fmt.Sprintf("postgres://postgres:events-test-only@127.0.0.1:%d/events?sslmode=disable", port)
			}
			db, err := sql.Open(engine, dsn)
			require.NoError(t, err)
			db.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			repo := New(db, engine, zerolog.Nop())
			require.NoError(t, repo.Migrate(t.Context()))
			facts := []catalogue.Facts{}
			add := func(id int64, name, date, status string) {
				var d *string
				if date != "" {
					d = &date
				}
				facts = append(facts, catalogue.Facts{PageID: id, ListRevision: 1, ArticleRevision: 1, RetrievedAt: time.Now().UTC(), Title: fmt.Sprintf("UFC %d", id), Name: name, Date: d, Type: "numbered", Status: status, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: fmt.Sprint(id), Current: true, Verified: true}}})
			}
			for i := 1; i <= 14; i++ {
				add(int64(i), fmt.Sprint("Completed ", i), fmt.Sprintf("2026-10-%02d", i), "completed")
			}
			add(100, "Later", "2027-01-01", "scheduled")
			add(101, "Sooner", "2026-11-15", "scheduled")
			add(102, "TBA", "", "scheduled")
			add(103, "Cancelled", "2026-11-16", "cancelled")
			add(104, "Unknown past", "2026-11-09", "unknown")
			add(105, "Stale scheduled", "2026-11-01", "scheduled")
			facts[15].Review = []string{"article_enrichment_unavailable"}
			_, err = repo.ApplyCatalogue(t.Context(), &catalogue.Snapshot{ListRevision: 1, Discovered: len(facts), Facts: facts})
			require.NoError(t, err)
			var oldID string
			require.NoError(t, db.QueryRow("SELECT id FROM ufc_events WHERE source_page_id=1").Scan(&oldID))
			old, err := repo.LoadCatalogueEvent(t.Context(), oldID)
			require.NoError(t, err)
			list, err := repo.CatalogueEvents(t.Context(), "2026-11-10")
			require.NoError(t, err)
			require.Len(t, list.Upcoming, 3)
			require.Equal(t, "Sooner", list.Upcoming[0].Name)
			require.True(t, list.Upcoming[0].NeedsReview)
			require.NotEmpty(t, list.Upcoming[0].Aliases)
			require.Equal(t, "Later", list.Upcoming[1].Name)
			require.Equal(t, "TBA", list.Upcoming[2].Name)
			require.Len(t, list.RecentCompleted, 10)
			for i, e := range list.RecentCompleted {
				require.Equal(t, fmt.Sprint("Completed ", 14-i), e.Name)
			}
			_, err = repo.ApplyCatalogue(t.Context(), &catalogue.Snapshot{ListRevision: 2, Discovered: 1, Facts: []catalogue.Facts{facts[14]}})
			require.NoError(t, err)
			after, err := repo.LoadCatalogueEvent(t.Context(), oldID)
			require.NoError(t, err)
			require.Greater(t, after.Quality.SettingsRevision, old.Quality.SettingsRevision)
			old.Quality.SettingsRevision = after.Quality.SettingsRevision
			require.Equal(t, old, after, "limited refresh must leave older IDs and records untouched")
			list, err = repo.CatalogueEvents(t.Context(), "2026-11-10")
			require.NoError(t, err)
			require.Len(t, list.RecentCompleted, 10)
			_, err = repo.CatalogueEvents(t.Context(), "invalid")
			require.Error(t, err)
		})
	}
}
