/*
 * Copyright (c) 2026, UFC Manager contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { RectangleStackIcon, Square3Stack3DIcon } from "@heroicons/react/24/outline";
import { Link, Outlet } from "@tanstack/react-router";

import { classNames } from "@utils";

const navigation = [
  { label: "Upcoming Events", to: "/ufc/events", icon: RectangleStackIcon },
  { label: "Past Events", to: "/ufc/past-events", icon: RectangleStackIcon },
  { label: "Library", to: "/ufc/library", icon: RectangleStackIcon },
  { label: "Downloads", to: "/ufc/downloads", icon: RectangleStackIcon },
  { label: "Testing", to: "/ufc/matching", icon: RectangleStackIcon },
  { label: "Settings", to: "/ufc/settings", icon: Square3Stack3DIcon }
] as const;

export function UFCLayout() {
  return (
    <main>
      <div className="mx-auto my-6 max-w-(--breakpoint-xl) px-4 sm:px-6 lg:px-8">
        <h1 className="text-3xl font-bold text-black dark:text-white">UFC Manager</h1>
      </div>
      <div className="mx-auto max-w-(--breakpoint-xl) px-2 pb-6 sm:px-6 lg:px-8 lg:pb-16">
        <div className="rounded-lg border border-gray-250 bg-white shadow-table dark:border-gray-775 dark:bg-gray-800">
          <div className="lg:grid lg:grid-cols-12">
            <aside className="border-b border-gray-150 py-2 lg:col-span-3 lg:border-r lg:border-b-0 dark:border-gray-725">
              <nav aria-label="UFC navigation" className="space-y-1">
                {navigation.map(item => (
                  <Link key={item.to} to={item.to} activeProps={{ "aria-current": "page" }}>
                    {({ isActive }) => <span className={classNames(
                      "group flex items-center border-l-4 px-3 py-2 text-sm font-medium transition",
                      isActive
                        ? "border-sky-500 bg-blue-100 font-bold text-sky-700 hover:bg-blue-200 hover:text-sky-900 dark:border-blue-500 dark:bg-gray-700 dark:text-gray-200 dark:hover:bg-gray-600 dark:hover:text-white"
                        : "border-transparent text-gray-900 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-gray-600 dark:hover:text-gray-300"
                    )}>
                      <item.icon aria-hidden="true" className="mr-3 -ml-1 h-6 w-6 shrink-0 text-gray-500 group-hover:text-gray-600 dark:text-gray-400 dark:group-hover:text-gray-300" />
                      {item.label}
                    </span>}
                  </Link>
                ))}
              </nav>
            </aside>
            <div className="min-w-0 lg:col-span-9 [&_input]:text-[13px] [&_select]:text-[13px] [&_textarea]:text-[13px]"><Outlet /></div>
          </div>
        </div>
      </div>
    </main>
  );
}
