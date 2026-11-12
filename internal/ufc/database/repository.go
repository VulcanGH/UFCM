// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package database persists UFC state on the existing application connection.
package database

import (
	"context"
	"database/sql"
	"embed"
	"math"

	"github.com/autobrr/autobrr/internal/ufc/foundation"
	"github.com/autobrr/autobrr/pkg/errors"
	"github.com/autobrr/autobrr/pkg/migrator"

	"github.com/rs/zerolog"
)

//go:embed migrations/*/*.sql
var migrations embed.FS

// Repository owns the isolated UFC schema and foundation settings.
type Repository struct {
	db     *sql.DB
	driver string
	log    zerolog.Logger
}

// New returns a repository using the application's existing database handle.
func New(db *sql.DB, driver string, log zerolog.Logger) *Repository {
	return &Repository{db: db, driver: driver, log: log}
}

// Migrate applies only UFC migrations and preserves upstream version history.
func (r *Repository) Migrate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.driver != "sqlite" && r.driver != "postgres" {
		return errors.New("unsupported UFC database driver %q", r.driver)
	}
	m := r.migrator()
	if err := m.InitVersionTable(); err != nil {
		return errors.Wrap(err, "could not initialise UFC migration history")
	}
	if err := r.checkHistory(ctx, m, false); err != nil {
		return err
	}
	if err := m.Migrate(); err != nil {
		return errors.Wrap(err, "could not migrate UFC schema")
	}
	return ctx.Err()
}

// LoadApplicationSettings reads the singleton foundation settings.
func (r *Repository) LoadApplicationSettings(ctx context.Context) (foundation.ApplicationSettings, error) {
	var settings foundation.ApplicationSettings
	if err := r.checkHistory(ctx, r.migrator(), true); err != nil {
		return settings, err
	}
	err := r.db.QueryRowContext(ctx, "SELECT revision, automatic_acquisition_enabled, processing_enabled FROM ufc_application_settings WHERE id = 1").Scan(&settings.Revision, &settings.AutomaticAcquisitionEnabled, &settings.ProcessingEnabled)
	if err != nil {
		return settings, errors.Wrap(err, "could not read UFC application settings")
	}
	return settings, nil
}

func (r *Repository) migrator() *migrator.Migrator {
	m := migrator.NewMigrate(r.db, migrator.WithEngine(r.driver), migrator.WithTableName("ufc_schema_migrations"), migrator.WithLegacyConversions(false), migrator.WithEmbedFS(migrations, "migrations/"+r.driver), migrator.WithLogger(r.log))
	m.AddFileMigration("0_application_settings.sql")
	m.AddFileMigration("2_event_catalogue.sql")
	m.AddFileMigration("3_manual_events_sync.sql")
	m.AddFileMigration("4_quality_preferences.sql")
	m.AddFileMigration("5_manual_acquisitions.sql")
	m.AddFileMigration("6_library_inventory.sql")
	m.AddFileMigration("7_permanent_catalogue.sql")
	m.AddFileMigration("8_download_visibility.sql")
	m.AddFileMigration("9_catalogue_identities.sql")
	m.AddFileMigration("10_manual_workflow.sql")
	m.AddFileMigration("11_processing_settings.sql")
	m.AddFileMigration("12_processing_jobs.sql")
	m.AddFileMigration("13_processing_recovery.sql")
	m.AddFileMigration("14_processing_file_components.sql")
	m.AddFileMigration("15_processing_cleanup.sql")
	m.AddFileMigration("16_processing_handoff.sql")
	m.AddFileMigration("17_automatic_acquisition.sql")
	m.AddFileMigration("18_application_updates.sql")
	return m
}

func (r *Repository) checkHistory(ctx context.Context, m *migrator.Migrator, requireCurrent bool) error {
	rows, err := r.db.QueryContext(ctx, "SELECT id, version FROM ufc_schema_migrations ORDER BY id")
	if err != nil {
		return errors.Wrap(err, "could not read UFC migration history")
	}
	defer rows.Close()
	expected := m.GetUpToId(math.MaxInt)
	applied := 0
	for rows.Next() {
		var id int
		var version string
		if err := rows.Scan(&id, &version); err != nil {
			return errors.Wrap(err, "could not read UFC migration version")
		}
		if applied >= len(expected) || expected[applied].Id() != id || expected[applied].Name != version {
			return errors.New("unsupported UFC migration history; restore a compatible database and binary")
		}
		applied++
	}
	if err := rows.Err(); err != nil {
		return errors.Wrap(err, "could not read UFC migration history")
	}
	if requireCurrent && applied != len(expected) {
		return errors.New("UFC schema migration is incomplete")
	}
	return nil
}
