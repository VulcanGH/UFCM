// Copyright (c) 2021 - 2025, Ludvig Lundgren and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build windows || darwin || linux

package main

//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --in ../../distrib/windows/ufcm-winres.json --arch amd64 --out rsrc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	_ "time/tzdata"

	"github.com/autobrr/autobrr/internal/application"
	"github.com/autobrr/autobrr/internal/cli"
	"github.com/autobrr/autobrr/internal/config"
	"github.com/autobrr/autobrr/internal/meta"
	"github.com/autobrr/autobrr/internal/product"
	"github.com/autobrr/autobrr/pkg/errors"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func init() {
	// the tray event loop must run on the main OS thread on macOS and Windows
	runtime.LockOSThread()

	// cobra refuses to run when started from Explorer unless this is cleared,
	// and the check runs inside Execute, before any RunE
	cobra.MousetrapHelpText = ""
}

func main() {
	meta.Set(version, commit, date)
	identity, identityErr := product.Selected()
	if identityErr != nil {
		showError("Application", identityErr.Error())
		os.Exit(1)
	}

	var configPath, updatedFrom string

	root := &cobra.Command{
		Use:          identity.Slug() + "-desktop",
		Short:        identity.Name() + " with a system tray icon",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), configPath, updatedFrom)
		},
	}

	root.PersistentFlags().StringVar(&configPath, "config", "", "path to configuration directory")

	// set by the restart after an update
	root.Flags().StringVar(&updatedFrom, strings.TrimPrefix(updatedFromFlag, "--"), "", "")
	_ = root.Flags().MarkHidden(strings.TrimPrefix(updatedFromFlag, "--"))

	root.AddCommand(cli.CommandVersion())
	if identity.IsUFC() {
		root.AddCommand(&cobra.Command{Use: "uninstall-cleanup", Hidden: true, Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { return cleanupInstalledLogin(identity.Slug()) }})
	}

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// run keeps the tray on the calling goroutine and the application on another.
// Whichever ends first takes the other down: Quit cancels ctx, and a returning
// Run removes the tray, which unblocks tray.Run. After an update it starts the
// new exe once the application has shut down.
func run(ctx context.Context, configPath, updatedFrom string) error {
	ctx, cancel := cli.ShutdownContext(ctx)
	defer cancel()

	app, err := newApp(configPath)
	if err != nil {
		showError("Application", err.Error())

		return err
	}

	name := app.Config().Product.Name()
	if app.Config().Product.IsUFC() {
		app.SetUpdateShutdown(cancel)
	}

	// checked before Run, so a second instance never opens the database
	unlock, locked, err := lockInstance(app.Config().ConfigPath)
	if err != nil {
		app.Log().Error().Err(err).Msg("could not check for a running instance")
		showError(name, "Could not establish the application lock. The database was not opened.")
		return err
	} else if !locked {
		showInfo(name, name+" is already running. Open it from the tray icon.")

		return nil
	}

	// resolved before an update renames the running exe
	exe, err := os.Executable()
	if err != nil {
		showError(name, err.Error())

		return errors.Wrap(err, "could not find executable")
	}

	var restartRequested atomic.Bool
	restart := func() {
		restartRequested.Store(true)
		cancel()
	}

	updater := newSelfUpdater(exe)

	tray := newTray(app, cancel, updater, restart, updatedFrom)

	var runErr error
	done := make(chan struct{})
	go func() {
		runErr = app.Run(ctx)
		close(done)
		tray.Remove()
	}()

	if err := onSessionEnd(func() {
		cancel()
		<-done
		updater.close()
	}); err != nil {
		app.Log().Error().Err(err).Msg("could not watch for session end")
	}

	if err := tray.Run(); err != nil {
		app.Log().Error().Err(err).Msg("tray stopped unexpectedly")
	}

	cancel()
	<-done
	updater.close()

	if runErr != nil {
		showError(name, fmt.Sprintf("%s stopped: %v\n\nLog file: %s", name, runErr, app.Config().LogPath))

		return runErr
	}

	if restartRequested.Load() {
		// the new process must be able to take the instance lock while this one exits
		if unlock != nil {
			unlock()
		}

		cmd := exec.Command(exe, restartArgs(os.Args[1:], meta.GetVersion())...)
		if err := cmd.Start(); err != nil {
			showError(name, fmt.Sprintf("%s was updated but could not restart: %v\n\nStart it again from the Start Menu.", name, err))

			return errors.Wrap(err, "could not restart")
		}
	}

	return nil
}

// newApp defaults to the OS config directory, because a desktop launch has no
// --config and its working directory is often read-only, and to a log file in
// it, because the GUI build has no console to log to.
func newApp(configPath string) (*application.App, error) {
	identity, err := product.Selected()
	if err != nil {
		return nil, err
	}
	if configPath == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return nil, errors.Wrap(err, "could not find config directory")
		}

		configPath = filepath.Join(dir, identity.Slug())
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	if cfg.Config.LogPath == "" {
		cfg.Config.LogPath = filepath.Join(configPath, "logs", identity.Slug()+".log")
	}

	return application.NewFromConfig(cfg), nil
}
