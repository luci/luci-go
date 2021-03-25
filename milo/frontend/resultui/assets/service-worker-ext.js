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

/*eslint-env serviceworker */

let cachedAccessToken = '';
let cachedUserId = null;
let timeout = 0;

self.addEventListener('message', (e) => {
  switch (e.data.type) {
    case 'SET_AUTH_RESPONSE': {
      cachedAccessToken = e.data.authResponse.access_token || '';
      cachedUserId = e.data.userId;
      clearTimeout(timeout);
      // Clear the accessToken and user ID 5 mins before it expires, so a small
      // delay won't cause the page to use an expired token.
      if (e.data.authResponse.expires_in !== undefined) {
        timeout = setTimeout(() => {
          cachedAccessToken = '';
          cachedUserId = null;
        }, (e.data.authResponse.expires_in - 300) * 1000);
      }
      break;
    }
    default:
      console.warn('unexpected message type');
  }
});

self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);
  if (url.pathname === '/ui/cached-access-token.js') {
    const res = new Response(
      `CACHED_ACCESS_TOKEN=${JSON.stringify(cachedAccessToken)};CACHED_USER_ID=${JSON.stringify(cachedUserId)};`
    );
    res.headers.set('content-type', 'application/javascript');
    e.respondWith(res);
  }
});
