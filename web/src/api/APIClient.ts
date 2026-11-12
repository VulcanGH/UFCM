/*
 * Copyright (c) 2021 - 2025, Ludvig Lundgren and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { baseUrl, sseBaseUrl } from "@utils";
import { GithubRelease } from "@app/types/Update";
import { AuthContext, AuthInfo } from "@utils/Context";
import { ColumnFilter } from "@tanstack/react-table";
import { IrcEvent } from "@hooks/useIrcEvents";

import type { UFCStatus, UFCEventList, UFCEvent, UFCEventEdit, UFCEventCreate, UFCCatalogueStatus, UFCSettings, UFCSettingsEdit } from "@app/types/UFC";

type RequestBody = BodyInit | object | Record<string, unknown> | null;
type Primitive = string | number | boolean | symbol | undefined;
type ValidateResponse = {
  username?: AuthInfo['username'];
  auth_method?: AuthInfo['authMethod'];
  profile_picture?: AuthInfo['profilePicture'];
}

interface HttpConfig {
  /**
   * One of "GET", "POST", "PUT", "PATCH", "DELETE", etc.
   * See https://developer.mozilla.org/en-US/docs/Web/HTTP/Methods
   */
  method?: string;
  /**
   * JSON body for this request. Once this is set to an object,
   * then `Content-Type` for this request is set to `application/json`
   * automatically.
   */
  body?: RequestBody;
  /**
   * Helper to work with a query string/search param of a URL.
   * E.g. ?a=1&b=2&c=3
   *
   * Using this interface will automatically convert
   * the object values into RFC-3986-compliant strings.
   *
   * Keys will *NOT* be sanitized, and any whitespace and
   * invalid characters will remain.
   *
   * The only supported value types are:
   * numbers, booleans, strings and flat 1-D arrays.
   *
   * Objects as values are not supported.
   *
   * The supported values are serialized as follows:
   *  - undefined values are ignored
   *  - empty strings are ignored
   *  - empty strings inside arrays are ignored
   *  - empty arrays are ignored
   *  - arrays append each time with the key and for each child
   *    e.g. `{ arr: [1, 2, 3] }` will yield `?arr=1&arr=2&arr=3`
   *  - array items with an undefined value (or which serialize to an empty string) are ignored,
   *    e.g. `{ arr: [1, undefined, undefined] }` will yield `?arr=1`
   *    (NaN, +Inf, -Inf, etc. will remain since they are valid serializations)
   */
  queryString?: Record<string, Primitive | Primitive[]>;
}

/**
 * Encodes a string into a RFC-3986-compliant string.
 *
 * By default, encodeURIComponent will not encode
 * any of the following characters: !'()*
 *
 * So a simple regex replace is done which will replace
 * these characters with their hex-value representation.
 *
 * @param str Input string (dictionary value).
 * @returns A RFC-3986-compliant string variation of the input string.
 * @note See https://stackoverflow.com/a/62969380
 */
function encodeRFC3986URIComponent(str: string): string {
  return encodeURIComponent(str).replace(
    /[!'()*]/g,
    (c) => `%${c.charCodeAt(0).toString(16).toUpperCase()}`
  );
}

/**
 * Makes a request on the network and returns a promise.
 *
 * This function serves as both a request builder and a response interceptor.
 *
 * @param endpoint The endpoint path relative to the backend instance.
 * @param config A dictionary which specifies what information this network
 * request must relay during transport. See @ref HttpClient.
 * @returns A promise for the *sent* network request which must *  be await'ed or .then()-chained before it can be used.
 *
 * If the status code returned by the server is in the [200, 300) range, then this is considered a success.
 *    - This function resolves with an empty dictionary object, i.e. {}, if the status code is 204 No data
 *    - The parsed JSON body is returned by this method if the server returns `Content-Type: application/json`.
 *    - In all other scenarios, the raw Response object from window.fetch() is returned,
 *      which must be handled manually by awaiting on one of its methods.
 *
 * The following is done if the status code that the server returns is NOT successful,
 * that is, if it falls outside of the [200, 300] range:
 *  - A unique Error object is returned if the user is logged in and the status code is 403 Forbidden.
 *    This Error object *should* be consumed by the @tanstack/query code, which indirectly calls HttpClient.
 *    The current user is then prompted to log in again after being logged out.
 *  - The `ErrorPage` screen appears in all other scenarios.
 */
export async function HttpClient<T = unknown>(
  endpoint: string,
  config: HttpConfig = {}
): Promise<T> {
  const init: RequestInit = {
    method: config.method,
    headers: { "Accept": "*/*", 'x-requested-with': 'XMLHttpRequest' },
    credentials: "include",
  };

  if (config.body) {
    init.body = JSON.stringify(config.body);

    if (typeof(config.body) === "object") {
      init.headers = {
        ...init.headers,
        "Content-Type": "application/json"
      };
    }
  }

  if (config.queryString) {
    const params: string[] = [];

    for (const [key, value] of Object.entries(config.queryString)) {
      const serializedKey = encodeRFC3986URIComponent(key);

      if (typeof(value) === "undefined") {
        // Skip case when the value is undefined.
        // The solution in this case is to use the request body instead with JSON
        continue;
      } else if (Array.isArray(value)) {
        // Append (don't set) each array member as a query parameter
        // e.g. ?a=1&a=2&a=3
        value.forEach((child) => {
          // Skip undefined member values
          const v = typeof(child) !== "undefined" ? String(child) : "";
          if (v.length) {
            params.push(`${serializedKey}=${encodeRFC3986URIComponent(v)}`);
          }
        });
      } else {
        // This is a primitive value, just add as string
        // e.g. ?a=1
        const v = String(value);
        if (v.length) {
          params.push(`${serializedKey}=${encodeRFC3986URIComponent(v)}`);
        }
      }
    }

    if (params.length) {
      endpoint += `?${params.join("&")}`;
    }
  }

  const response = await window.fetch(`${baseUrl()}${endpoint}`, init);

  const isJson = response.headers.get("Content-Type")?.includes("application/json");

  if (response.status >= 200 && response.status < 300) {
    // We received a successful response
    if (response.status === 204) {
      // 204 contains no data, but indicates success
      return Promise.resolve<T>({} as T);
    }

    // If Content-Type is application/json, then parse response as JSON
    // otherwise, just resolve the Response object returned by window.fetch
    // and the consumer can call await response.text() if needed.
    if (isJson) {
      return Promise.resolve<T>(await response.json() as T);
    } else {
      return Promise.resolve<T>(response as T);
    }
  } else {
    // This is not a successful response.
    // It is most likely an error.
    switch (response.status) {
    case 403: {
      // Plain-text 403s come from the auth middleware (session gone); JSON 403s
      // are application errors and fall through to show the server's message.
      if (!isJson && AuthContext.get().isLoggedIn) {
        return Promise.reject(new Error("Cookie expired or invalid."));
      }
      break;
    }
    case 500: {
      const health = await window.fetch(`${baseUrl()}api/healthz/liveness`);
      if (!health.ok) {
        return Promise.reject(
          new Error(`[500] Offline (Internal server error): "${endpoint}"`)
        );
      }
      break;
    }
    case 503: {
      // Show an error toast to notify the user what occurred
      return Promise.reject(new Error(`[503] Service unavailable: "${endpoint}"`));
    }
    default:
      break;
    }

    let reason = response.statusText;
    if (isJson) {
      const json = await response.json();
      if (Object.hasOwn(json, "message")) {
        reason = json.message as string;
      }
    }

    if (reason.length) {
      reason = ` (${reason})`;
    }

    const defaultError = new Error(
      `HTTP request to '${endpoint}' failed with code ${response.status}${reason}`
    );
    return Promise.reject(Object.assign(defaultError, { status: response.status }));
  }
}

const appClient = {
  Get: <T>(endpoint: string, config: HttpConfig = {}) => HttpClient<T>(endpoint, {
    ...config,
    method: "GET"
  }),
  Post: <T = void>(endpoint: string, config: HttpConfig = {}) => HttpClient<T>(endpoint, {
    ...config,
    method: "POST"
  }),
  Put: <T = void>(endpoint: string, config: HttpConfig = {}) => HttpClient<T>(endpoint, {
    ...config,
    method: "PUT"
  }),
  Patch: (endpoint: string, config: HttpConfig = {}) => HttpClient<void>(endpoint, {
    ...config,
    method: "PATCH"
  }),
  Delete: (endpoint: string, config: HttpConfig = {}) => HttpClient<void>(endpoint, {
    ...config,
    method: "DELETE"
  })
};


export const APIClient = {
  ufc: {
    updateStatus: () => appClient.Get<import("@app/types/UFCUpdate").UFCUpdateStatus>("api/ufc/updates/"),
    checkUpdate: () => appClient.Post<import("@app/types/UFCUpdate").UFCUpdateStatus>("api/ufc/updates/check", {}),
    installUpdate: (id: string) => appClient.Post<import("@app/types/UFCUpdate").UFCUpdateStatus>("api/ufc/updates/install", {body: {id}}),
    saveUpdatePreferences: (revision: number, auto_install: boolean) => appClient.Put<import("@app/types/UFCUpdate").UFCUpdateStatus>("api/ufc/updates/settings", {body: {revision, auto_install}}),
 automationSettings:()=>appClient.Get<{revision:number;enabled:boolean}>("api/ufc/automation/settings"),
 saveAutomationSettings:(input:{revision:number;enabled:boolean})=>appClient.Put<{revision:number;enabled:boolean}>("api/ufc/automation/settings",{body:input}),
    processingSettings: () => appClient.Get<import("@app/types/UFCProcessing").UFCProcessingSettings>("api/ufc/processing/settings"),
    previewProcessing: (acquisitionID: string) => appClient.Post<import("@app/types/UFCProcessing").UFCProcessingPreview>("api/ufc/processing/preview", { body: { acquisition_id: acquisitionID } }),
    queueProcessing: (acquisitionID: string) => appClient.Post<import("@app/types/UFCProcessing").UFCProcessingJob>("api/ufc/processing/jobs", { body: { acquisition_id: acquisitionID } }),
    processingJobs: () => appClient.Get<import("@app/types/UFCProcessing").UFCProcessingJob[]>("api/ufc/processing/jobs"),
    processingReview: (id: string) => appClient.Get<import("@app/types/UFCProcessing").UFCProcessingReview>(`api/ufc/processing/jobs/${encodeURIComponent(id)}/review`),
    retryProcessing: (id: string) => appClient.Post<import("@app/types/UFCProcessing").UFCProcessingJob>(`api/ufc/processing/jobs/${encodeURIComponent(id)}/retry`),
    retryProcessingCleanup: (id: string) => appClient.Post<import("@app/types/UFCProcessing").UFCProcessingJob>(`api/ufc/processing/jobs/${encodeURIComponent(id)}/cleanup/retry`),
    keepBothProcessing: (id: string, destination: string) => appClient.Post<import("@app/types/UFCProcessing").UFCProcessingJob>(`api/ufc/processing/jobs/${encodeURIComponent(id)}/keep-both`, { body: { destination } }),
    skipProcessing: (id: string) => appClient.Post<import("@app/types/UFCProcessing").UFCProcessingJob>(`api/ufc/processing/jobs/${encodeURIComponent(id)}/skip`),
    cancelProcessing: (id: string) => appClient.Post<import("@app/types/UFCProcessing").UFCProcessingJob>(`api/ufc/processing/jobs/${encodeURIComponent(id)}/cancel`),
    saveProcessingSettings: (input: import("@app/types/UFCProcessing").UFCProcessingSettings) => appClient.Put<import("@app/types/UFCProcessing").UFCProcessingSettings>("api/ufc/processing/settings", { body: input }),
    sabCategories: (clientID: number) => appClient.Get<string[]>(`api/ufc/processing/categories/${clientID}`),
    library: () => appClient.Get<import("@app/types/UFCLibrary").UFCLibraryView>("api/ufc/library"),
    librarySettings: () => appClient.Get<import("@app/types/UFCLibrary").UFCLibrarySettings>("api/ufc/library/settings"),
    saveLibrarySettings: (input: { revision: number; paths: string[] }) => appClient.Put<import("@app/types/UFCLibrary").UFCLibrarySettings>("api/ufc/library/settings", { body: input }),
    scanLibrary: () => appClient.Post<{ run_id: string }>("api/ufc/library/scan", { body: {} }),
    clearLibrary: (revision: number) => appClient.Post<{ cleared: boolean }>("api/ufc/library/clear", { body: { revision } }),
    libraryFiles: (group: string, offset: number) => appClient.Get<{ files: import("@app/types/UFCLibrary").UFCLibraryFile[]; total: number }>(`api/ufc/library/files?${new URLSearchParams({ group, offset: String(offset) })}`),
    editLibraryFile: (id: string, input: import("@app/types/UFCLibrary").UFCLibraryFileEdit) => HttpClient<import("@app/types/UFCLibrary").UFCLibraryFile>(`api/ufc/library/files/${encodeURIComponent(id)}`, { method: "PATCH", body: input }),
    libraryEvents: () => appClient.Get<{ id: string; name: string; date: string | null }[]>("api/ufc/library/events"),
    libraryCoverage: () => appClient.Get<import("@app/types/UFCLibrary").UFCLibraryCoverage>("api/ufc/library/coverage"),
    mainCardTerms: () => appClient.Get<{ name: string; terms: string[] }[]>("api/ufc/matching/terms"),
    recoverDownload: (id: string, input: { revision: number; remote_id: string; confirm: boolean }) => appClient.Post<import("@app/types/UFCDownload").UFCDownload>(`api/ufc/downloads/${encodeURIComponent(id)}/recover`, { body: input }),
    downloadClients: () => appClient.Get<import("@app/types/UFCDownload").UFCDownloadClient[]>("api/ufc/downloads/clients"),
    downloads: () => appClient.Get<import("@app/types/UFCDownload").UFCDownload[]>("api/ufc/downloads"),
    removeDownload: (id: string) => appClient.Delete(`api/ufc/downloads/${encodeURIComponent(id)}`),
    catalogueEvents: (search: string, offset: number, limit = 20) => appClient.Get<{ events: import("@app/types/UFC").UFCEventSummary[]; total: number }>(`api/ufc/catalogue/events?${new URLSearchParams({ search, offset: String(offset), limit: String(limit), past: "1" })}`),
    submitDownload: (input: import("@app/types/UFCDownload").UFCDownloadRequest) => appClient.Post<import("@app/types/UFCDownload").UFCDownloadAccepted>("api/ufc/downloads", { body: input }),
    reconcileDownloads: () => appClient.Post<{ refreshed: boolean }>("api/ufc/downloads/reconcile", { body: {} }),
    searchSources: () => appClient.Get<import("@app/types/UFCSearch").UFCSearchSource[]>("api/ufc/search/sources"),
    searchEvent: (id: string, input: import("@app/types/UFCSearch").UFCSearchRequest) => appClient.Post<import("@app/types/UFCSearch").UFCSearchResponse>(`api/ufc/events/${encodeURIComponent(id)}/search`, { body: input }),
    previewMatch: (title: string) => appClient.Post<import("@app/types/UFCMatching").UFCMatchingPreview>("api/ufc/matching/preview", { body: { title } }),
    clearEvents: () => HttpClient<{ cleared: boolean }>("api/ufc/events", { method: "DELETE" }),
    status: () => appClient.Get<UFCStatus>("api/ufc/status"),
    settings: () => appClient.Get<UFCSettings>("api/ufc/catalogue/settings"),
    saveSettings: (edit: UFCSettingsEdit) => appClient.Put<UFCSettings>("api/ufc/catalogue/settings", { body: edit }),
    events: () => appClient.Get<UFCEventList>("api/ufc/events"),
    event: (id: string) => appClient.Get<UFCEvent>(`api/ufc/events/${encodeURIComponent(id)}`),
    createEvent: (input: UFCEventCreate) => appClient.Post<UFCEvent>("api/ufc/events", { body: input }),
    deleteEvent: (id: string, revision: number) => HttpClient<UFCEvent>(`api/ufc/events/${encodeURIComponent(id)}`, { method: "PATCH", body: { revision, delete: true } }),
    editEvent: (id: string, edit: UFCEventEdit) => HttpClient<UFCEvent>(`api/ufc/events/${encodeURIComponent(id)}`, { method: "PATCH", body: edit }),
    catalogueStatus: () => appClient.Get<UFCCatalogueStatus>("api/ufc/catalogue/status"),
    refreshCatalogue: (scope: "upcoming" | "past" | "rebuild" = "upcoming", confirm = false) => appClient.Post<{ coalesced: boolean }>("api/ufc/catalogue/refresh", { body: { scope, confirm } }),
    resetCatalogue: () => appClient.Post<{ reset: boolean }>("api/ufc/catalogue/reset", { body: { confirm: true } })
  },
  auth: {
    login: (username: string, password: string, remember_me: boolean) => appClient.Post("api/auth/login", {
      body: { username, password, remember_me }
    }),
    logout: () => appClient.Post("api/auth/logout"),
    validate: async (): Promise<ValidateResponse> => {
        return await appClient.Get<ValidateResponse>("api/auth/validate");
    },
    onboard: (username: string, password: string) => appClient.Post("api/auth/onboard", {
      body: { username, password }
    }),
    canOnboard: () => appClient.Get("api/auth/onboard"),
    updateUser: (req: UserUpdate) => appClient.Patch(`api/auth/user/${req.username_current}`,
      { body: req }),
    getOIDCConfig: async (redirect?: string) => {
      try {
        return await appClient.Get<{ enabled: boolean; autoRedirect: boolean; authorizationUrl: string; state: string; disableBuiltInLogin: boolean; issuerUrl: string }>("api/auth/oidc/config", {
          queryString: redirect ? { redirect } : undefined
        });
      } catch (error: unknown) {
        if (error instanceof Error && error.message?.includes('404')) {
          return { enabled: false, autoRedirect: false, authorizationUrl: '', state: '', disableBuiltInLogin: false, issuerUrl: '' };
        }
        throw error;
      }
    },
  },
  actions: {
    create: (action: Action) => appClient.Post("api/actions", {
      body: action
    }),
    update: (action: Action) => appClient.Put(`api/actions/${action.id}`, {
      body: action
    }),
    delete: (id: number) => appClient.Delete(`api/actions/${id}`),
    toggleEnable: (id: number) => appClient.Patch(`api/actions/${id}/toggleEnabled`)
  },
  apikeys: {
    getAll: () => appClient.Get<APIKey[]>("api/keys"),
    create: (key: APIKey) => appClient.Post<APIKey>("api/keys", {
      body: key
    }),
    update: (key: APIKey) => appClient.Put(`api/keys/${key.id}`, {
      body: key
    }),
    delete: (id: number) => appClient.Delete(`api/keys/${id}`)
  },
  config: {
    get: () => appClient.Get<Config>("api/config"),
    update: (config: ConfigUpdate) => appClient.Patch("api/config", {
      body: config
    })
  },
  downloaders: {
    getAll: () => appClient.Get<Downloader[]>("api/downloaders"),
    getArrTags: (clientID: number) => appClient.Get<ArrTag[]>(`api/downloaders/${clientID}/arr/tags`),
    create: (dc: Downloader) => appClient.Post("api/downloaders", {
      body: dc
    }),
    update: (dc: Downloader) => appClient.Put("api/downloaders", {
      body: dc
    }),
    delete: (id: number) => appClient.Delete(`api/downloaders/${id}`),
    test: (dc: Downloader) => appClient.Post("api/downloaders/test", {
      body: dc
    })
  },
  filters: {
    getAll: () => appClient.Get<Filter[]>("api/filters"),
    find: (indexers: string[], sortOrder: string) => appClient.Get<Filter[]>("api/filters", {
      queryString: {
        sort: sortOrder,
        indexer: indexers
      }
    }),
    getByID: (id: number) => appClient.Get<Filter>(`api/filters/${id}`),
    create: (filter: Filter) => appClient.Post<Filter>("api/filters", {
      body: filter
    }),
    update: (filter: Filter) => appClient.Put<Filter>(`api/filters/${filter.id}`, {
      body: filter
    }),
    duplicate: (id: number) => appClient.Post<Filter>(`api/filters/${id}/duplicate`),
    toggleEnable: (id: number, enabled: boolean) => appClient.Put(`api/filters/${id}/enabled`, {
      body: { enabled }
    }),
    pruneDeprecatedIndexers: (identifiers: string[] = []) => appClient.Post<{ removed: number }>("api/filters/indexers/prune-deprecated", {
      body: { identifiers }
    }),
    delete: (id: number) => appClient.Delete(`api/filters/${id}`),
    external: {
      test: (external: ExternalFilter) => appClient.Post<ExternalFilterTestResult>("api/filters/external/test", {
        body: external
      })
    },
    notifications: {
      get: (filterId: number) => appClient.Get<FilterNotification[]>(`api/filters/${filterId}/notifications`),
      update: (filterId: number, notifications: FilterNotification[]) => 
        appClient.Put(`api/filters/${filterId}/notifications`, {
          body: notifications
        })
    }
  },
  feeds: {
    find: () => appClient.Get<Feed[]>("api/feeds"),
    create: (feed: FeedCreate) => appClient.Post("api/feeds", {
      body: feed
    }),
    fetchCapsDraft: (feed: FeedCapsRequest) => appClient.Post<unknown>("api/feeds/caps", {
      body: feed
    }),
    fetchCaps: (id: number) => appClient.Get<FeedCaps>(`api/feeds/${id}/caps`),
    toggleEnable: (id: number, enabled: boolean) => appClient.Patch(`api/feeds/${id}/enabled`, {
      body: { enabled }
    }),
    update: (feed: Feed) => appClient.Put(`api/feeds/${feed.id}`, {
      body: feed
    }),
    forceRun: (id: number) => appClient.Post(`api/feeds/${id}/forcerun`),
    delete: (id: number) => appClient.Delete(`api/feeds/${id}`),
    deleteCache: (id: number) => appClient.Delete(`api/feeds/${id}/cache`),
    test: (feed: Feed) => appClient.Post("api/feeds/test", {
      body: feed
    })
  },
  indexers: {
    // returns all configured indexer rows (including archived/deprecated ones, flagged)
    getOptions: () => appClient.Get<Indexer[]>("api/indexer/options"),
    // returns indexer definitions for all currently present/enabled indexers
    getAll: () => appClient.Get<IndexerDefinition[]>("api/indexer"),
    // returns metadata for removed/deprecated indexers
    getDeprecations: () => appClient.Get<IndexerDeprecation[]>("api/indexer/deprecations"),
    deleteArchived: (id: number) => appClient.Delete(`api/indexer/deprecations/${id}`),
    // returns all possible indexer definitions
    getSchema: () => appClient.Get<IndexerDefinition[]>("api/indexer/schema"),
    create: (indexer: Indexer) => appClient.Post<Indexer>("api/indexer", {
      body: indexer
    }),
    update: (indexer: Indexer) => appClient.Put(`api/indexer/${indexer.id}`, {
      body: indexer
    }),
    delete: (id: number) => appClient.Delete(`api/indexer/${id}`),
    testApi: (req: IndexerTestApiReq) => appClient.Post<IndexerTestApiReq>(`api/indexer/${req.id}/api/test`, {
      body: req
    }),
    toggleEnable: (id: number, enabled: boolean) => appClient.Patch(`api/indexer/${id}/enabled`, {
      body: { enabled }
    })
  },
  irc: {
    getNetworks: () => appClient.Get<IrcNetworkWithHealth[]>("api/irc"),
    createNetwork: (network: IrcNetworkCreate) => appClient.Post("api/irc", {
      body: network
    }),
    updateNetwork: (network: IrcNetwork) => appClient.Put(`api/irc/network/${network.id}`, {
      body: network
    }),
    deleteNetwork: (id: number) => appClient.Delete(`api/irc/network/${id}`),
    restartNetwork: (id: number) => appClient.Post(`api/irc/network/${id}/restart`),
    sendCmd: (cmd: SendIrcCmdRequest) => appClient.Post(`api/irc/network/${cmd.network_id}/cmd`, {
      body: cmd
    }),
    reprocessAnnounce: (networkId: number, channel: string, msg: string) => appClient.Post(`api/irc/network/${networkId}/channel/${channel}/announce/process`, {
      body: { msg: msg }
    }),
    getChannelHistory: (networkId: number, channel: string, limit?: number) =>
      appClient.Get<IrcEvent[]>(`api/irc/network/${networkId}/channel/${channel}/history`, {
        queryString: { limit }
      }),
    allEvents: () => new EventSource(
      `${sseBaseUrl()}api/irc/events?stream=irc`,
      { withCredentials: true }
    )
  },
  matrix: {
    list: () => appClient.Get<MatrixNetwork[]>("api/matrix"),
    getByID: (id: number) => appClient.Get<MatrixNetwork>(`api/matrix/network/${id}`),
    store: (network: MatrixNetworkCreate) => appClient.Post<MatrixNetwork>("api/matrix", {
      body: network
    }),
    update: (network: MatrixNetwork) => appClient.Put(`api/matrix/network/${network.id}`, {
      body: network
    }),
    delete: (id: number) => appClient.Delete(`api/matrix/network/${id}`),
    restart: (id: number) => appClient.Post(`api/matrix/network/${id}/restart`),
    login: (id: number) => appClient.Post<MatrixLogin>(`api/matrix/network/${id}/login`),
    setAccessToken: (id: number, accessToken: string) => appClient.Post(`api/matrix/network/${id}/token`, {
      body: { access_token: accessToken }
    }),
    logout: (id: number) => appClient.Post(`api/matrix/network/${id}/logout`),
    getRoomHistory: (networkId: number, room: string) => appClient.Get<MatrixMessage[]>(`api/matrix/network/${networkId}/room/history`, {
      queryString: { room }
    }),
    reprocessAnnounce: (networkId: number, room: string, msg: string) => appClient.Post(`api/matrix/network/${networkId}/announce/process`, {
      body: { room, msg }
    }),
    events: () => new EventSource(
      `${sseBaseUrl()}api/matrix/events?stream=matrix`,
      { withCredentials: true }
    )
  },
  logs: {
    files: () => appClient.Get<LogFileResponse>("api/logs/files"),
    download: (filename: string) => appClient.Get<Response>(`api/logs/files/${encodeURIComponent(filename)}`)
  },
  alerts: {
    list: () => appClient.Get<Alert[]>("api/alerts")
  },
  events: {
    logs: () => new EventSource(`${sseBaseUrl()}api/events?stream=logs`, { withCredentials: true }),
    notifications: () => new EventSource(`${sseBaseUrl()}api/events?stream=notifications`, { withCredentials: true })
  },
  notifications: {
    getAll: () => appClient.Get<ServiceNotification[]>("api/notification"),
    create: (notification: ServiceNotification) => appClient.Post("api/notification", {
      body: notification
    }),
    update: (notification: ServiceNotification) => appClient.Put(
      `api/notification/${notification.id}`,
      { body: notification }
    ),
    delete: (id: number) => appClient.Delete(`api/notification/${id}`),
    test: (notification: ServiceNotification) => appClient.Post("api/notification/test", {
      body: notification
    }),
    getPushoverSounds: (apiToken: string) => {
      // Don't make request if token is redacted or empty
      if (!apiToken || apiToken === "<redacted>" || apiToken === "") {
        return Promise.reject(new Error("API token is required"));
      }
      return appClient.Get<Record<string, string>>(`api/notification/pushover/sounds?token=${encodeURIComponent(apiToken)}`);
    },
    inbox: {
      list: (params: InboxQueryParams) => appClient.Get<InboxResponse>("api/notification/inbox", {
        queryString: { limit: params.limit, offset: params.offset, unread: params.unread, event: params.event }
      }),
      markRead: (ids: number[] = []) => appClient.Post("api/notification/inbox/read", {
        body: { ids }
      }),
      delete: (ids: number[]) => appClient.Post("api/notification/inbox/delete", {
        body: { ids }
      }),
      deleteAll: () => appClient.Delete("api/notification/inbox")
    }
  },
  lists: {
    list: () => appClient.Get<List[]>("api/lists"),
    getByID: (id: number) => appClient.Get<List>(`api/lists/${id}`),
    store: (list: List) => appClient.Post("api/lists", {
      body: list
    }),
    update: (list: List) => appClient.Put(`api/lists/${list.id}`, {
      body: list
    }),
    delete: (id: number) => appClient.Delete(`api/lists/${id}`),
    refreshList: (id: number) => appClient.Post(`api/lists/${id}/refresh`),
    refreshAll: () => appClient.Post(`api/lists/refresh`),
    test: (list: List) => appClient.Post("api/lists/test", {
      body: list
    })
  },
  proxy: {
    list: () => appClient.Get<Proxy[]>("api/proxy"),
    getByID: (id: number) => appClient.Get<Proxy>(`api/proxy/${id}`),
    store: (proxy: ProxyCreate) => appClient.Post("api/proxy", {
      body: proxy
    }),
    update: (proxy: Proxy) => appClient.Put(`api/proxy/${proxy.id}`, {
      body: proxy
    }),
    delete: (id: number) => appClient.Delete(`api/proxy/${id}`),
    usage: (id: number) => appClient.Get<ProxyUsage>(`api/proxy/${id}/usage`),
    test: (proxy: Proxy) => appClient.Post("api/proxy/test", {
      body: proxy
    })
  },
  release: {
    find: (query?: string) => appClient.Get<ReleaseFindResponse>(`api/release${query}`),
    findRecent: () => appClient.Get<ReleaseFindResponse>("api/release/recent"),
    findQuery: (offset?: number, limit?: number, filters?: ColumnFilter[]) => {
      const params: Record<string, string[]> = {
        indexer: [],
        push_status: [],
        q: []
      };

      filters?.forEach((filter) => {
        if (!filter.value)
          return;

        if (filter.id == "indexer.identifier" || filter.id == "indexer_identifier") {
          if (typeof filter.value === "string") {
            params["indexer"].push(filter.value);
          } else if (Array.isArray(filter.value)) {
            for (const v of filter.value) {
              if (typeof v === "string") params["indexer"].push(v);
            }
          }
        } else if (filter.id === "action_status") {
          if (typeof filter.value === "string") {
            params["push_status"].push(filter.value);
          } // push_status is the correct value here otherwise the releases table won't load when filtered by push status
        } else if (filter.id === "push_status") {
          if (typeof filter.value === "string") {
            params["push_status"].push(filter.value);
          }
        } else if (filter.id == "name") {
          if (typeof filter.value === "string") {
            params["q"].push(filter.value);
          }
        }
      });

      return appClient.Get<ReleaseFindResponse>("api/release", {
        queryString: {
          offset,
          limit,
          ...params
        }
      });
    },
    indexerOptions: () => appClient.Get<string[]>("api/release/indexers"),
    stats: () => appClient.Get<ReleaseStats>("api/release/stats"),
    statsActivity: (days: number) => appClient.Get<ReleaseActivityStats>("api/release/stats/activity", {
      queryString: { days }
    }),
    statsVolume: (days: number) => appClient.Get<ReleaseVolumeStats>("api/release/stats/volume", {
      queryString: { days }
    }),
    statsHeatmap: (days: number) => appClient.Get<ReleaseHeatmapStats>("api/release/stats/heatmap", {
      queryString: { days }
    }),
    statsTopIndexers: (days: number) => appClient.Get<ReleaseTopIndexersStats>("api/release/stats/indexers", {
      queryString: { days }
    }),
    statsTopFilters: (days: number) => appClient.Get<ReleaseTopFiltersStats>("api/release/stats/filters", {
      queryString: { days }
    }),
    delete: (params: DeleteParams) => {
      return appClient.Delete("api/release", {
        queryString: {
          olderThan: params.olderThan,
          indexer: params.indexers,
          releaseStatus: params.releaseStatuses,
        }
      });
    },
    replayAction: (releaseId: number, actionId: number) => appClient.Post(
      `api/release/${releaseId}/actions/${actionId}/retry`
    ),
    cleanupJobs: {
      list: () => appClient.Get<ReleaseCleanupJob[]>("api/release/cleanup-jobs"),
      getByID: (id: number) => appClient.Get<ReleaseCleanupJob>(`api/release/cleanup-jobs/${id}`),
      store: (job: ReleaseCleanupJob) => appClient.Post("api/release/cleanup-jobs", {
        body: job
      }),
      update: (job: ReleaseCleanupJob) => appClient.Put(`api/release/cleanup-jobs/${job.id}`, {
        body: job
      }),
      delete: (id: number) => appClient.Delete(`api/release/cleanup-jobs/${id}`),
      toggleEnabled: (id: number, enabled: boolean) => appClient.Patch(
        `api/release/cleanup-jobs/${id}/enabled`,
        { body: { enabled } }
      ),
      forceRun: (id: number) => appClient.Post(`api/release/cleanup-jobs/${id}/run`)
    },
    profiles: {
      duplicates: {
        list: () => appClient.Get<ReleaseProfileDuplicate[]>(`api/release/profiles/duplicate`),
        delete: (id: number) => appClient.Delete(`api/release/profiles/duplicate/${id}`),
        store: (profile: ReleaseProfileDuplicate) => appClient.Post(`api/release/profiles/duplicate`, {
          body: profile
        }),
      }
    }
  },
  updates: {
    check: () => appClient.Get("api/updates/check"),
    getLatestRelease: () => appClient.Get<GithubRelease>("api/updates/latest")
  }
};
