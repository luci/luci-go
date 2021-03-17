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

// Eslint isn't able to type-check webworker scripts.
/* eslint-disable */

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);
  const isNewBuildPage =
    // Short build link.
    url.pathname.match(/^\/b\/[^/]+(\/.*)?/) ||
    // Long build link.
    url.pathname.match(/^\/p\/[^/]+\/builders\/[^/]+\/[^/]+\/[^/]+(\/.*)?/);
  if (isNewBuildPage) {
    url.pathname = '/ui' + url.pathname;
    event.respondWith(Response.redirect(url.toString()));
    return;
  }
  event.respondWith(fetch(event.request));
});
