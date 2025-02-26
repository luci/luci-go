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

import 'virtual:ui_version.js';
import 'virtual:settings.js';
import 'virtual:override-milo-host';

import { cleanupOutdatedCaches, precacheAndRoute } from 'workbox-precaching';
import { NavigationRoute, registerRoute } from 'workbox-routing';
import { NetworkOnly } from 'workbox-strategies';

import { Prefetcher } from '@/common/sw_utils/prefetch';

import { createHandlerBoundToURL } from './stale_while_revalidate';

// Tell TSC that this is a ServiceWorker script.
declare const self: ServiceWorkerGlobalScope;

/**
 * A regex for the defined routes.
 */
declare const DEFINED_ROUTES_REGEXP: string;

// Unconditionally skip waiting so the clients can always get the newest version
// when the page is refreshed. The only downside is that clients on an old
// version may encounter errors when lazy loading cached static assets. This is
// not critical because
// 1. This is unlikely to happen since all JS assets are prefetched.
// 2. All lazy-loadable assets (i.e. excluding entry files) have content hashes
//    in their filenames. This means in case of a cache miss,
//    1. it's virtually impossible to lazy load an asset of an incompatible
//       version because the content hash won't match, and
//    2. they can have a much longer cache duration (currently configured to be
//       4 weeks), the asset loading request can likely be fulfilled by other
//       cache layers (e.g. AppEngine server cache, browser HTTP cache).
// 3. When the client failed to lazy load an asset, a simple refresh will be
//    able to fix the issue anyway.
self.skipWaiting();

const prefetcher = new Prefetcher(SETTINGS, self.fetch.bind(self));

self.addEventListener('fetch', async (e) => {
  if (e.request.mode === 'navigate') {
    const url = new URL(e.request.url);
    prefetcher.prefetchResources(url.pathname);
    return;
  }

  if (prefetcher.respondWithPrefetched(e)) {
    return;
  }

  // The version of the UI is the AppEngine UI service version that installed
  // this service worker. Not the current active AppEngine UI service version.
  if (e.request.url === self.origin + '/ui_version.js') {
    const res = new Response(`
      self.UI_VERSION = '${UI_VERSION}';
      self.UI_VERSION_TYPE = '${UI_VERSION_TYPE}';
    `);
    res.headers.set('content-type', 'text/javascript');
    e.respondWith(res);
    return;
  }

  // Ensure all clients served by this service worker use the same settings.
  if (e.request.url === self.origin + '/settings.js') {
    const res = new Response(
      `self.SETTINGS = Object.freeze(${JSON.stringify(SETTINGS)});\n`,
    );
    res.headers.set('content-type', 'text/javascript');
    e.respondWith(res);
    return;
  }
});

// Enable workbox specific features AFTER we registered our own event handlers
// to make sure our own event handlers have the chance to intercept the
// requests.
{
  self.addEventListener('message', (event) => {
    // This is not used currently. We keep it here anyway to ensure that calling
    // `Workbox.prototype.messageSkipWaiting` is not a noop should it be used in
    // the future.
    if (event.data?.type === 'SKIP_WAITING') {
      self.skipWaiting();
    }
  });

  cleanupOutdatedCaches();
  precacheAndRoute(self.__WB_MANIFEST, {
    // Let NavigationRoute handler handles /ui/ route so our
    // stale-while-revalidating logic works.
    //
    // Use type casting because the type declaration is invalid. `null` should
    // be accepted as a valid value here.
    // See https://developer.chrome.com/docs/workbox/modules/workbox-precaching#directory_index
    directoryIndex: null as unknown as undefined,
  });

  registerRoute(
    new NavigationRoute(
      createHandlerBoundToURL('/ui/index.html', {
        // Do not serve cache if the service worker is more than one week old.
        // This ensures that any version we deployed to prod will be purged
        // after one week. Without this, any version can be cached effectively
        // indefinitely because purging that version requires each user to visit
        // LUCI UI (on each hosted domain) at least once, and the first visit
        // since the last update still uses the old version.
        staleWhileRevalidate: 7 * 24 * 60 * 60 * 1000,
      }),
      {
        // Only handle defined routes so when the user visits a newly added
        // route, the service worker won't serve an old cache, causing the user
        // to see a 404 page until the new version is activated.
        allowlist: [new RegExp(DEFINED_ROUTES_REGEXP, 'i')],
      },
    ),
  );

  // Do not intercept cypress related routes.
  // Otherwise, the cypress test may not work.
  registerRoute(
    (request) => request.url.pathname.startsWith('/__cypress/'),
    new NetworkOnly(),
  );
}
