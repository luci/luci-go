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

import { cached } from './libs/cached_fn';
import { PrpcClientExt } from './libs/prpc_client_ext';
import { timeout } from './libs/utils';
import { BuildsService, GetBuildRequest } from './services/buildbucket';

importScripts('/configs.js');

// TSC isn't able to determine the scope properly.
// Perform manual casting to fix typing.
const _self = (self as unknown) as ServiceWorkerGlobalScope;

const AUTH_STATE_KEY = 'auth-state';

export interface SetAuthStateEventData {
  type: 'SET_AUTH_STATE';
  authState: AuthState | null;
}

const cachedFetch = cached(
  // _expiresIn is not used here but is used in the expire function below.
  // _expiresIn is listed here to help TSC generates the correct type
  // definition.
  (info: RequestInfo, init: RequestInit | undefined, _expiresIn: number) => fetch(info, init),
  {
    key: (info, init) => {
      const req = new Request(info, init);
      let body: string | undefined = undefined;
      if (req.body) {
        body = init?.body?.toString();
        // If we can't get the string representation of the body, we can't
        // compare the request with others. Return a unique key.
        if (body === undefined) {
          return {};
        }
      }
      return JSON.stringify({
        ...req,
        headers: [...req.headers].sort(([k1, v1], [k2, v2]) => k1.localeCompare(k2) || v1.localeCompare(v2)),
        body,
      });
    },
    expire: ([, , expiresIn]) => timeout(expiresIn),
  }
);

_self.addEventListener('message', async (e) => {
  switch (e.data.type) {
    case 'SET_AUTH_STATE': {
      const data = e.data as SetAuthStateEventData;
      await kvSet(AUTH_STATE_KEY, data.authState);
      break;
    }
    default:
      console.warn('unexpected message type', e.data.type, e.data, e);
  }
});

_self.addEventListener('fetch', async (e) => {
  const url = new URL(e.request.url);
  // Serve cached auth data.
  if (url.pathname === '/ui/cached-auth-state.js') {
    e.respondWith(
      (async () => {
        const authState = (await kvGet<AuthState | null>(AUTH_STATE_KEY)) || null;
        return new Response(`CACHED_AUTH_STATE=${JSON.stringify(authState)};`, {
          headers: { 'content-type': 'application/javascript' },
        });
      })()
    );
  }

  // Ensure all clients served by this service worker use the same config.
  if (url.pathname === '/configs.js') {
    const res = new Response(`var CONFIGS=${JSON.stringify(CONFIGS)};`);
    res.headers.set('content-type', 'application/javascript');
    e.respondWith(res);
  }

  prefetchResources(url);

  // Response to build request from cache.
  const shouldUseCache = e.request.url === `https://${CONFIGS.BUILDBUCKET.HOST}/prpc/buildbucket.v2.Builds/GetBuild`;
  if (shouldUseCache) {
    e.respondWith(
      (async () => {
        const res = await cachedFetch(
          // The response can't be reused, don't keep it in cache.
          { skipUpdate: true, invalidateCache: true },
          e.request,
          { body: await e.request.clone().text() },
          0
        );
        return res;
      })()
    );
  }
});

/**
 * Prefetches some resources if the url matches certain condition.
 */
async function prefetchResources(reqUrl: URL) {
  // Prefetch the build.
  const match = reqUrl.pathname.match(/^\/ui\/p\/([^/]+)\/builders\/([^/]+)\/([^/]+)\/(b?\d+)\/?/i);
  if (!match) {
    return;
  }
  const [, project, bucket, builder, buildIdOrNum] = match as string[];
  const req: GetBuildRequest = buildIdOrNum.startsWith('b')
    ? { id: buildIdOrNum.slice(1), fields: '*' }
    : { builder: { project, bucket, builder }, buildNumber: Number(buildIdOrNum), fields: '*' };

  const authState = (await kvGet<AuthState | null>(AUTH_STATE_KEY)) || null;
  const prefetchBuildsService = new BuildsService(
    new PrpcClientExt(
      {
        host: CONFIGS.BUILDBUCKET.HOST,
        fetchImpl: async (info: RequestInfo, init?: RequestInit) => {
          // Because build info is time sensitive, don't keep it too long.
          await cachedFetch({}, info, init, 5000);

          // Abort the function to prevent the response from being consumed.
          throw 0;
        },
      },
      () => authState?.accessToken || ''
    )
  );

  // Bypass the service cache but trigger the cachedFetch cache.
  prefetchBuildsService
    .getBuild(req, { acceptCache: false, skipUpdate: true })
    // Ignore any error, let the consumer of the cache deal with it.
    .catch((_e) => {});
}
