// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package application

import (
	"context"
	"fmt"

	"github.com/autobrr/autobrr/internal/ufc/updater"
)

// SetUpdateShutdown connects launcher-managed updates before Run starts.
func (app *App) SetUpdateShutdown(shutdown func()) { app.updateShutdown = shutdown }

// UFCUpdateStatus waits for application readiness before accessing its update service.
func (app *App) UFCUpdateStatus(ctx context.Context) (updater.Status, error) {
	select {
	case <-ctx.Done():
		return updater.Status{}, ctx.Err()
	case <-app.ready:
	}
	if app.ufcUpdateService == nil {
		return updater.Status{}, fmt.Errorf("updates unavailable")
	}
	return app.ufcUpdateService.Status(ctx)
}

// UFCCheckUpdate uses the same coordinator as the authenticated web action.
func (app *App) UFCCheckUpdate(ctx context.Context) error {
	if _, err := app.UFCUpdateStatus(ctx); err != nil {
		return err
	}
	_, err := app.ufcUpdateService.Check(ctx)
	return err
}

// UFCInstallUpdate requests the currently verified release without a second confirmation.
func (app *App) UFCInstallUpdate(ctx context.Context) error {
	status, err := app.UFCUpdateStatus(ctx)
	if err != nil {
		return err
	}
	_, err = app.ufcUpdateService.Install(ctx, status.ID)
	return err
}
