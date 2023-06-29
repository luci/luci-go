// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * @fileoverview
 * By default, new service worker will not be activated until all clients are
 * disconnected (i.e. all browser tabs are closed). Because refreshing clients
 * doesn't disconnect the clients, it could be difficult for the users to
 * discover the way to receive bug fixes in the new version if the users are on
 * a broken release.
 *
 * This file adds a mechanism to force activating the service worker.
 */

import { get as kvGet, set as kvSet } from 'idb-keyval';

// Tell TSC that this is a ServiceWorker script.
declare const self: ServiceWorkerGlobalScope;

const VERSION_KEY = `@/common/service_workers/version_management/VERSION_KEY-${self.registration.scope}`;

export interface VersionUpdateEventData {
  readonly type: 'VERSION_UPDATE';
  readonly version: number;
}

export interface EnsureVersionOpts {
  readonly version: string;
  readonly shouldSkipWaiting: (lastActivatedVersion?: string) => boolean;
}

/**
 * Force activating the service worker if the version of the last activated
 * service worker is not in the specified version range.
 *
 * Once the service worker is activated, update the recorded version number to
 * the newly provided number.
 */
export function init(opts: EnsureVersionOpts) {
  self.addEventListener('install', (e) => {
    e.waitUntil(
      kvGet(VERSION_KEY).then((lastActivatedVersion) => {
        const shouldSkipWaiting =
          typeof lastActivatedVersion !== 'string' ||
          opts.shouldSkipWaiting(lastActivatedVersion);

        if (shouldSkipWaiting) {
          self.skipWaiting();
        }
      })
    );
  });

  self.addEventListener('activate', (e) => {
    e.waitUntil(kvSet(VERSION_KEY, opts.version));
  });
}
