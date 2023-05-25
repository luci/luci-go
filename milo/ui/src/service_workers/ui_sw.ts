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

import 'virtual:configs.js';
import { cleanupOutdatedCaches, createHandlerBoundToURL, precacheAndRoute } from 'workbox-precaching';
import { NavigationRoute, registerRoute } from 'workbox-routing';

import './force_update';
import { Prefetcher } from './prefetch';

// Tell TSC that this is a ServiceWorker script.
declare const self: ServiceWorkerGlobalScope & { CONFIGS: typeof CONFIGS };

cleanupOutdatedCaches();
precacheAndRoute(self.__WB_MANIFEST);

self.addEventListener('message', (event) => {
  if (event.data?.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }
});

registerRoute(new NavigationRoute(createHandlerBoundToURL('/ui/index.html'), { allowlist: [/^\/ui\//] }));

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
    const res = new Response(`self.CONFIGS=Object.freeze(${JSON.stringify(CONFIGS)});`);
    res.headers.set('content-type', 'application/javascript');
    e.respondWith(res);
    return;
  }

  prefetcher.prefetchResources(url);
});
