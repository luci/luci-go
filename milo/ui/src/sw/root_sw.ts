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
 * This service worker redirects milo links to ResultUI links.
 * This is 200-400ms faster than redirecting on the server side.
 */

// Tell TSC that this is a ServiceWorker script.
declare const self: ServiceWorkerGlobalScope;
// Add a dummy export so signal this file a module.
// Otherwise TSC won't allow us to re-declare `self`.
export {};

self.addEventListener('fetch', (e) => {
  if (e.request.mode !== 'navigate') {
    return;
  }

  const url = new URL(e.request.url);

  // The `/ui` route is not intercepted by the service worker registered at
  // `/ui/`. Redirects to `/ui/` so the page load can take advantage of the
  // optimizations offered by the service worker registered at `/ui/`.
  if (url.pathname === '/ui') {
    url.pathname = '/ui/';
    e.respondWith(Response.redirect(url.toString()));
    return;
  }

  const isSPA =
    // Project selector.
    url.pathname === '/' ||
    // Short build link.
    url.pathname.match(/^\/b\//) ||
    // Long build link.
    url.pathname.match(/^\/p\/[^/]+\/builders\/[^/]+\/[^/]+\//) ||
    // Consoles link.
    url.pathname.match(/^\/p\/[^/]+(\/)?$/) ||
    // Builders link.
    url.pathname.match(/^\/p\/[^/]+(\/g\/[^/]+)?\/builders(\/)?$/) ||
    // Invocation link.
    url.pathname.match(/^\/inv\//) ||
    // Artifact link.
    url.pathname.match(/^\/artifact\//) ||
    // Search page.
    url.pathname.match(/^\/search(\/|$)/);

  if (isSPA) {
    url.pathname = '/ui' + url.pathname;
    e.respondWith(Response.redirect(url.toString()));
    return;
  }
});

// Ensures that the redirection logic takes effect immediately so users won't be
// redirected to broken pages (e.g. when app is rolled back to a previous
// version).
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()));
