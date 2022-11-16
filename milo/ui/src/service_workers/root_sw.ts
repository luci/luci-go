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

// TSC isn't able to determine the scope properly.
// Perform manual casting to fix typing.
const _self = self as unknown as ServiceWorkerGlobalScope;

// TODO(crbug/1108198): we don't need this after removing the /ui prefix.
_self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);

  const isResultUI =
    // Short build link.
    url.pathname.match(/^\/b\//) ||
    // Long build link.
    url.pathname.match(/^\/p\/[^/]+\/builders\/[^/]+\/[^/]+\//) ||
    // Builders link.
    url.pathname.match(/^\/p\/[^/]+(\/g\/[^/]+)?\/builders(\/)?$/) ||
    // Invocation link.
    url.pathname.match(/^\/inv\//) ||
    // Artifact link.
    url.pathname.match(/^\/artifact\//) ||
    // Search page.
    url.pathname.match(/^\/search(\/|$)/);

  if (isResultUI) {
    url.pathname = '/ui' + url.pathname;
    event.respondWith(Response.redirect(url.toString()));
    return;
  }
});

// Ensures that the redirection logic takes effect immediately so users won't be
// redirected to broken pages (e.g. when app is rolled back to a previous
// version).
_self.addEventListener('install', () => _self.skipWaiting());
_self.addEventListener('activate', (e) => e.waitUntil(_self.clients.claim()));
