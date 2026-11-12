/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */
import { useState, type ButtonHTMLAttributes } from "react";

const colourClasses = new Set(["text-gray-600", "dark:text-gray-300", "hover:text-blue-600", "dark:hover:text-blue-500", "focus-visible:text-blue-600"]);

export function UFCActionButton({ busy = false, className = "", onClick, onPointerEnter, onPointerLeave, disabled, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { busy?: boolean }) {
  const [activated, setActivated] = useState(false);
  const colour = busy && activated ? "text-blue-600 dark:text-blue-500" : `text-gray-600 dark:text-gray-300${activated ? "" : " hover:text-blue-600 dark:hover:text-blue-500"}`;
  return <button {...props} disabled={disabled || busy} aria-busy={busy} className={`${className.split(" ").filter(value => !colourClasses.has(value)).join(" ")} ${colour} focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500`}
    onClick={event => { setActivated(true); onClick?.(event); }}
    onPointerEnter={event => { setActivated(busy); onPointerEnter?.(event); }}
    onPointerLeave={event => { if (!busy) setActivated(false); onPointerLeave?.(event); }} />;
}
