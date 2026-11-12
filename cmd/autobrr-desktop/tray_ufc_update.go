// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later
//go:build windows || darwin || linux

package main

import (
	"context"
	"time"

	"github.com/autobrr/autobrr/internal/application"

	"github.com/gogpu/systray"
)

func addUFCUpdateMenu(menu *systray.Menu, app *application.App, tray *systray.SystemTray) {
	check := menu.Add("Check for Updates", func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := app.UFCCheckUpdate(ctx); err != nil {
				tray.ShowNotification("UFC Manager", "Could not check for updates. See Settings for details.")
			}
		}()
	})
	check.SetDisabled(true)
	install := menu.Add("Update and Restart", func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := app.UFCInstallUpdate(ctx); err != nil {
				tray.ShowNotification("UFC Manager", err.Error())
			}
		}()
	})
	install.SetDisabled(true)
	go func() {
		<-app.Ready()
		check.SetDisabled(false)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		notified := ""
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			status, err := app.UFCUpdateStatus(ctx)
			cancel()
			if err != nil {
				return
			}
			install.SetDisabled(!status.Supported || (status.Phase != "verified" && status.Phase != "deferred"))
			if status.Version != "" && status.ID != notified && (status.Phase == "verified" || status.Phase == "available") {
				notified = status.ID
				tray.ShowNotification("UFC Manager", "Update available: "+status.Version)
			}
		}
	}()
}
