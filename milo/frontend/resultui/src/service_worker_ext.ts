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

const _self = (self as unknown) as ServiceWorkerGlobalScope;

export interface SetAuthDataEventData {
  type: 'SET_AUTH_DATA';
  authResponse: gapi.auth2.AuthResponse;
  userId: string | null;
}

let cachedAccessToken = '';
let cachedUserId: string | null = null;
let timeout = 0;

_self.addEventListener('message', (e) => {
  switch (e.data.type) {
    case 'SET_AUTH_DATA': {
      const data = e.data as SetAuthDataEventData;
      cachedAccessToken = data.authResponse.access_token || '';
      cachedUserId = data.userId;
      clearTimeout(timeout);
      if (data.authResponse.expires_in !== undefined) {
        timeout = _self.setTimeout(
          () => {
            cachedAccessToken = '';
            cachedUserId = null;
          },

          // Clear the accessToken and user ID 3 min before it expires, so a
          // small delay won't cause the page to use an expired token.
          (e.data.authResponse.expires_in - 180) * 1000
        );
      }
      break;
    }
    default:
      console.warn('unexpected message type', e.data.type, e.data, e);
  }
});

_self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);
  if (url.pathname === '/ui/cached-auth-data.js') {
    const res = new Response(
      `CACHED_ACCESS_TOKEN=${JSON.stringify(cachedAccessToken)};CACHED_USER_ID=${JSON.stringify(cachedUserId)};`
    );
    res.headers.set('content-type', 'application/javascript');
    e.respondWith(res);
  }
});
