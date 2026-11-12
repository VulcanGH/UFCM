/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

export interface UFCUpdateStatus {
  revision: number;
  auto_install: boolean;
  id: string;
  version: string;
  phase: string;
  message: string;
  current: string;
  supported: boolean;
  reason: string;
  release_url: string;
}
