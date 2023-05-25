// Copyright 2021 The LUCI Authors.
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
 * This file adds a mechanism to force activating the service worker. When we
 * wants to purge an old broken version from user's browser, we can update the
 * FORCE_UPDATE_TOKEN_VALUE. The new service worker will skip waiting when the
 * token value doesn't match the value set by the previous service worker.
 */

import { get as kvGet, set as kvSet } from 'idb-keyval';

// Tell TSC that this is a ServiceWorker script.
declare const self: ServiceWorkerGlobalScope;

const FORCE_UPDATE_TOKEN_KEY = 'force-update-token-' + self.registration.scope;

// Update this value to ensure older versions are removed from the cache.
const FORCE_UPDATE_TOKEN_VALUE = 'v11';

self.addEventListener('install', (e) => {
  e.waitUntil(
    kvGet(FORCE_UPDATE_TOKEN_KEY).then((token) => {
      if (token !== FORCE_UPDATE_TOKEN_VALUE) {
        self.skipWaiting();
      }
    })
  );
});

self.addEventListener('activate', (e) => {
  e.waitUntil(kvSet(FORCE_UPDATE_TOKEN_KEY, FORCE_UPDATE_TOKEN_VALUE));
});
