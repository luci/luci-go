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

import { getAuthState, setAuthState } from './auth_state';
import { Prefetcher } from './prefetch';

importScripts('/configs.js');

// TSC isn't able to determine the scope properly.
// Perform manual casting to fix typing.
const _self = (self as unknown) as ServiceWorkerGlobalScope;

export interface SetAuthStateEventData {
  type: 'SET_AUTH_STATE';
  authState: AuthState | null;
}

_self.addEventListener('message', async (e) => {
  switch (e.data.type) {
    case 'SET_AUTH_STATE': {
      const data = e.data as SetAuthStateEventData;
      setAuthState(data.authState);
      break;
    }
    default:
      console.warn('unexpected message type', e.data.type, e.data, e);
  }
});

const prefetcher = new Prefetcher(CONFIGS, _self.fetch.bind(_self));

_self.addEventListener('fetch', async (e) => {
  if (prefetcher.respondWithPrefetched(e)) {
    return;
  }

  const url = new URL(e.request.url);
  // Serve cached auth data.
  if (url.pathname === '/ui/cached-auth-state.js') {
    e.respondWith(
      (async () => {
        const authState = await getAuthState();
        return new Response(`const CACHED_AUTH_STATE=${JSON.stringify(authState)};`, {
          headers: { 'content-type': 'application/javascript' },
        });
      })()
    );
  }

  // Ensure all clients served by this service worker use the same config.
  if (url.pathname === '/configs.js') {
    const res = new Response(`const CONFIGS=${JSON.stringify(CONFIGS)};`);
    res.headers.set('content-type', 'application/javascript');
    e.respondWith(res);
  }

  prefetcher.prefetchResources(url);
});
