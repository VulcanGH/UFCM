package database

import (
	"path/filepath"
	"testing"
	"time"

	upstream "github.com/autobrr/autobrr/internal/database"
	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/notifications"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func robustnessRepository(t *testing.T) *Repository {
	t.Helper()
	log := zerolog.Nop()
	db, err := upstream.NewDB(&domain.Config{DatabaseType: "sqlite", DatabaseDSN: filepath.Join(t.TempDir(), "ufc.db"), DatabaseAutoMigrate: true}, log)
	require.NoError(t, err)
	require.NoError(t, db.Open())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	repo := New(db.Handler, "sqlite", log)
	require.NoError(t, repo.Migrate(t.Context()))
	return repo
}

func TestResolveSavedCatalogueClash(t *testing.T) {
	ctx := t.Context()
	repo := robustnessRepository(t)
	date := "2024-04-13"
	alias := catalogue.Alias{Namespace: "ufc.fight_night", Value: "297", Current: true, Verified: true}
	first := catalogue.Facts{PageID: 100, ArticleRevision: 11, ListRevision: 20, RetrievedAt: time.Now().UTC(), Title: "UFC Fight Night 297", Name: "UFC Fight Night 297", Type: "fight_night", Date: &date, Status: "completed", Section: "past events", Aliases: []catalogue.Alias{alias}}
	second := first
	second.PageID, second.Title, second.Name = 101, "UFC Fight Night: Alpha vs. Beta", "UFC Fight Night: Alpha vs. Beta"
	second.Review = []string{"article_list_date_conflict"}
	second.ReviewEvidence = map[string][]string{"article_list_date_conflict": {"2024-04-13", "2024-04-14"}}
	snapshot := &catalogue.Snapshot{ListRevision: 20, Discovered: 2, Facts: []catalogue.Facts{first, second}}
	_, err := repo.ApplyCatalogue(ctx, snapshot)
	require.NoError(t, err)
	get := func(pageID int64) catalogue.Event {
		t.Helper()
		var id string
		require.NoError(t, repo.db.QueryRowContext(ctx, "SELECT id FROM ufc_events WHERE source_page_id=$1", pageID).Scan(&id))
		event, err := repo.LoadCatalogueEvent(ctx, id)
		require.NoError(t, err)
		return event
	}

	old, keep := get(100), get(101)
	require.Contains(t, old.Review, identifierConflict)
	require.Contains(t, keep.Review, identifierConflict)
	choice := catalogue.ReviewChoice{AliasKey: catalogue.KeyForAlias(alias), Keep: keep.ID, Records: []catalogue.ReviewRecord{{ID: old.ID, Revision: old.Revision}, {ID: keep.ID, Revision: keep.Revision}}}
	outdated := choice
	outdated.Records = append([]catalogue.ReviewRecord{}, choice.Records...)
	outdated.Records[0].Revision--
	require.ErrorIs(t, repo.ResolveAliasConflict(ctx, keep.ID, outdated, "test"), catalogue.ErrRevisionConflict)
	require.NoError(t, repo.ResolveAliasConflict(ctx, keep.ID, choice, "test"))
	require.Empty(t, get(100).Review)
	kept := get(101)
	require.Equal(t, []string{"article_list_date_conflict"}, kept.Review)
	_, err = repo.EditCatalogueEvent(ctx, kept.ID, catalogue.EventEdit{Revision: kept.Revision, Date: &catalogue.FieldEdit{Operation: "set", Value: &date}}, "test")
	require.NoError(t, err)
	require.Empty(t, get(101).Review)
	_, err = repo.ApplyCatalogue(ctx, snapshot)
	require.NoError(t, err)
	require.Empty(t, get(100).Review)
	require.Empty(t, get(101).Review)
	require.Empty(t, catalogue.ConflictingAliases([]catalogue.Event{get(100), get(101)}, kept.ID))
	second.ReviewEvidence["article_list_date_conflict"] = []string{"2024-04-13", "2024-04-15"}
	snapshot.Facts = []catalogue.Facts{second}
	_, err = repo.ApplyCatalogue(ctx, snapshot)
	require.NoError(t, err)
	require.Equal(t, []string{"article_list_date_conflict"}, get(101).Review)
}

func TestUFCDiscordCutoverAndDisabledEvent(t *testing.T) {
	repo := robustnessRepository(t)
	ctx := t.Context()
	legacy := []domain.Notification{{Type: domain.NotificationTypeDiscord, Enabled: true, Webhook: "https://discord.com/api/webhooks/123/fake-token", Events: []string{"UFC_COPY_COMPLETED"}}}
	delivery, err := notifications.New(ctx, repo, legacy, zerolog.Nop())
	require.NoError(t, err)
	settings, err := delivery.Settings(ctx)
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.True(t, settings.Configured)
	require.True(t, settings.Events[domain.NotificationEventUFCCopyCompleted].Enabled)
	require.False(t, settings.Events[domain.NotificationEventUFCAttention].Enabled)
	require.NoError(t, delivery.Deliver(ctx, domain.NotificationPayload{Event: domain.NotificationEventUFCAttention}))
	history, err := delivery.History(ctx)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "skipped", history[0].Outcome)
	_, err = notifications.New(ctx, repo, nil, zerolog.Nop())
	require.NoError(t, err)
	saved, err := delivery.Settings(ctx)
	require.NoError(t, err)
	require.Equal(t, settings, saved)
}
