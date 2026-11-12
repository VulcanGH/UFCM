// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	appconfig "github.com/autobrr/autobrr/internal/config"
	"github.com/autobrr/autobrr/internal/database"
	"github.com/autobrr/autobrr/internal/ufc/updater"

	"github.com/rs/zerolog"
	"golang.org/x/sys/windows"
)

func main() {
	if err := run(); err != nil {
		for _, arg := range os.Args[1:] {
			if strings.HasPrefix(arg, "--maintenance-") {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		title, _ := windows.UTF16PtrFromString("UFC Manager")
		message, _ := windows.UTF16PtrFromString(err.Error() + "\n\nYour database and library have been retained.")
		windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
		os.Exit(1)
	}
}

func run() error {
	var config string
	var maintenanceBegin, maintenanceEnd bool
	var installOverride, lease, cacheOverride string
	flag.StringVar(&config, "config", "", "configuration directory")
	flag.BoolVar(&maintenanceBegin, "maintenance-begin", false, "reserve installer maintenance")
	flag.BoolVar(&maintenanceEnd, "maintenance-end", false, "finish installer maintenance")
	flag.StringVar(&installOverride, "maintenance-install-dir", "", "installer target directory")
	flag.StringVar(&lease, "maintenance-lease", "", "installer product identity")
	flag.StringVar(&cacheOverride, "maintenance-cache-dir", "", "installer user cache directory")
	flag.Parse()
	if maintenanceBegin && maintenanceEnd {
		return fmt.Errorf("choose one installer maintenance action")
	}
	if flag.NArg() != 0 {
		return fmt.Errorf("unsupported launcher arguments")
	}
	if config == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		config = filepath.Join(base, "ufc-manager")
	}
	config, err := filepath.Abs(config)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	install := filepath.Dir(exe)
	if maintenanceBegin || maintenanceEnd {
		if !filepath.IsAbs(installOverride) {
			return fmt.Errorf("absolute installer target required")
		}
		install = filepath.Clean(installOverride)
	} else if installOverride != "" || lease != "" {
		return fmt.Errorf("installer arguments require maintenance mode")
	}
	local, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	if cacheOverride != "" {
		if (!maintenanceBegin && !maintenanceEnd) || !filepath.IsAbs(cacheOverride) {
			return fmt.Errorf("invalid installer cache directory")
		}
		local = filepath.Clean(cacheOverride)
	}
	sum := sha256.Sum256([]byte(strings.ToLower(install)))
	root := filepath.Join(local, "UFC Manager Runtime", hex.EncodeToString(sum[:8]))
	if maintenanceEnd {
		return updater.EndMaintenance(root, lease)
	}
	if maintenanceBegin {
		var state updater.Bootstrap
		if err := updater.ReadJSON(filepath.Join(root, "bootstrap-state.json"), &state); err == nil && state.Config != "" {
			config = state.Config
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	cfg, err := appconfig.Load(config)
	if err != nil {
		return err
	}
	db, err := database.NewDB(cfg.Config, zerolog.Nop())
	if err != nil {
		return err
	}
	preflight, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	err = updater.ValidateDatabaseStorage(preflight, db.Driver, db.DSN, root)
	if err == nil && maintenanceBegin {
		err = updater.ValidateDatabaseStorage(preflight, db.Driver, db.DSN, install)
	}
	cancel()
	if err != nil {
		return err
	}
	if maintenanceBegin {
		return updater.BeginMaintenance(root, config, lease)
	}
	trust, _ := updater.ProductionTrust()
	launcher := updater.Launcher{Root: root, Install: install, Config: config, Executable: exe, Trust: trust}
	return launcher.Run(context.Background())
}
