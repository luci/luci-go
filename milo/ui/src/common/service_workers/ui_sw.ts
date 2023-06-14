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

// TODO(weiweilin): add integration tests to ensure the SW works properly.

// This is injected by Vite.
// eslint-disable-next-line import/no-unresolved
import 'virtual:configs.js';
import {
  cleanupOutdatedCaches,
  createHandlerBoundToURL,
  precacheAndRoute,
} from 'workbox-precaching';
import { NavigationRoute, registerRoute } from 'workbox-routing';

import { parseVersion } from '../libs/version';

import { Prefetcher } from './prefetch';
import * as versionManagement from './version_management';

// Tell TSC that this is a ServiceWorker script.
declare const self: ServiceWorkerGlobalScope & { CONFIGS: typeof CONFIGS };

// Update the minimum version (exclusive) to force purging the old caches when
// releasing a critical bug fix.
const MIN_VERSION = '13381-5572ca8';

versionManagement.init({
  version: self.CONFIGS.VERSION,
  // Set the maximum version to the current version to ensure the new version
  // are always purged after reverts.
  shouldSkipWaiting: (lastActivatedVersion) => {
    if (!lastActivatedVersion) {
      return true;
    }

    const lastVer = parseVersion(lastActivatedVersion);
    const minVer = parseVersion(MIN_VERSION);
    const currentVer = parseVersion(self.CONFIGS.VERSION);

    // Default to purge the last version if any of the versions cannot be
    // parsed.
    if (!lastVer || !minVer || !currentVer) {
      return true;
    }

    // Ensures the service worker is newer than the minimum version.
    if (lastVer.num <= minVer.num) {
      return true;
    }

    // Ensures the service worker is always rolled back in case of a revert.
    if (currentVer.num < lastVer.num) {
      return true;
    }

    return false;
  },
});

/**
 * Whether the UI service worker should skip waiting.
 * Injected by Vite.
 */
declare const UI_SW_SKIP_WAITING: boolean;
if (UI_SW_SKIP_WAITING) {
  self.skipWaiting();
}

const prefetcher = new Prefetcher(self.CONFIGS, self.fetch.bind(self));

self.addEventListener('fetch', async (e) => {
  if (prefetcher.respondWithPrefetched(e)) {
    return;
  }

  const url = new URL(e.request.url);

  // Ensure all clients served by this service worker use the same config.
  if (url.pathname === '/configs.js') {
    const res = new Response(
      `self.CONFIGS=Object.freeze(${JSON.stringify(CONFIGS)});`
    );
    res.headers.set('content-type', 'application/javascript');
    e.respondWith(res);
    return;
  }

  prefetcher.prefetchResources(url);
});

// Enable workbox specific features AFTER we registered our own event handlers
// to make sure our own event handlers have the chance to intercept the
// requests.
{
  self.addEventListener('message', (event) => {
    if (event.data?.type === 'SKIP_WAITING') {
      self.skipWaiting();
    }
  });

  cleanupOutdatedCaches();
  precacheAndRoute(self.__WB_MANIFEST);
  registerRoute(
    new NavigationRoute(createHandlerBoundToURL('/ui/index.html'), {
      allowlist: [/^\/ui\//],
    })
  );
}
