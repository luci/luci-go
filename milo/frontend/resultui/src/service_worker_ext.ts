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

import { get as kvGet, set as kvSet } from 'idb-keyval';

// TSC isn't able to determine the scope properly.
// Perform manual casting to fix typing.
const _self = (self as unknown) as ServiceWorkerGlobalScope;

const AUTH_DATA_KEY = 'auth-data';

interface AuthState {
  accessToken: string;
  userId: string;
  expiresAt: number;
}

export interface SetAuthDataEventData {
  type: 'SET_AUTH_DATA';
  authResponse: gapi.auth2.AuthResponse;
  userId: string | null;
}

_self.addEventListener('message', async (e) => {
  switch (e.data.type) {
    case 'SET_AUTH_DATA': {
      const data = e.data as SetAuthDataEventData;
      if (!data.authResponse.expires_at || !data.authResponse.access_token || !data.userId) {
        await kvSet(AUTH_DATA_KEY, undefined);
      } else {
        await kvSet(AUTH_DATA_KEY, {
          accessToken: data.authResponse.access_token,
          userId: data.userId,
          // Expires the access token a bit earlier so a small delay won't cause
          // expired token being
          expiresAt: (data.authResponse.expires_at - 180) * 1000,
        });
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
    e.respondWith(
      (async () => {
        const cachedAuth = await kvGet<AuthState>(AUTH_DATA_KEY);
        const { accessToken, userId } =
          (cachedAuth?.expiresAt || 0) > Date.now() ? cachedAuth! : { accessToken: '', userId: null };

        return new Response(
          `CACHED_ACCESS_TOKEN=${JSON.stringify(accessToken)};CACHED_USER_ID=${JSON.stringify(userId)};`,
          { headers: { 'content-type': 'application/javascript' } }
        );
      })()
    );
  }
});
