/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */
import { UFCCompactToggle } from "./UFCCompact";

export function UFCQualityChoices({ values, onChange, disabled = false }: { values: string[]; onChange: (values: string[]) => void; disabled?: boolean }) {
  return <div className="flex flex-wrap gap-x-5 gap-y-2">{["2160p", "1080p", "1080i", "720p", "480p"].map(value => <UFCCompactToggle key={value} label={value} checked={values.includes(value)} disabled={disabled} onChange={checked => onChange(checked ? [...values, value] : values.filter(item => item !== value))} />)}</div>;
}
