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

import { get as kvGet, set as kvSet } from 'idb-keyval';

import { cached } from './libs/cached_fn';
import { PrpcClientExt } from './libs/prpc_client_ext';
import { genCacheKeyForPrpcRequest } from './libs/prpc_utils';
import { timeout } from './libs/utils';
import { BUILD_FIELD_MASK, BuildsService, GetBuildRequest } from './services/buildbucket';
import { ResultDb } from './services/resultdb';

importScripts('/configs.js');

const CACHED_PRPC_URLS = [
  `https://${CONFIGS.BUILDBUCKET.HOST}/prpc/buildbucket.v2.Builds/GetBuild`,
  `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/GetArtifact`,
];

// TSC isn't able to determine the scope properly.
// Perform manual casting to fix typing.
const _self = (self as unknown) as ServiceWorkerGlobalScope;

const AUTH_STATE_KEY = 'auth-state';
const PRPC_CACHE_KEY_PREFIX = 'prpc-cache-key';

export interface SetAuthStateEventData {
  type: 'SET_AUTH_STATE';
  authState: AuthState | null;
}

const cachedFetch = cached(
  // _cacheKey and _expiresIn are not used here but is used in the expire
  // and key functions below.
  // they are listed here to help TSC generates the correct type definition.
  (info: RequestInfo, init: RequestInit | undefined, _cacheKey: unknown, _expiresIn: number) => fetch(info, init),
  {
    key: (_info, _init, cacheKey) => cacheKey,
    expire: ([, , , expiresIn]) => timeout(expiresIn),
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
  if (CACHED_PRPC_URLS.includes(e.request.url)) {
    e.respondWith(
      (async () => {
        const res = await cachedFetch(
          // The response can't be reused, don't keep it in cache.
          { skipUpdate: true, invalidateCache: true },
          e.request,
          undefined,
          await genCacheKeyForPrpcRequest(PRPC_CACHE_KEY_PREFIX, e.request.clone()),
          0
        );
        return res;
      })()
    );
  }
});

/**
 * Prefetches resources if the url matches certain pattern.
 */
function prefetchResources(reqUrl: URL) {
  prefetchBuild(reqUrl);
  prefetchArtifact(reqUrl);
}

/**
 * Prefetches the build if the url matches certain pattern.
 */
async function prefetchBuild(reqUrl: URL) {
  let req: GetBuildRequest | null = null;
  let match = reqUrl.pathname.match(/^\/ui\/p\/([^/]+)\/builders\/([^/]+)\/([^/]+)\/(b?\d+)\/?/i);
  if (match) {
    const [project, bucket, builder, buildIdOrNum] = match.slice(1, 5).map((v) => decodeURIComponent(v));
    req = buildIdOrNum.startsWith('b')
      ? { id: buildIdOrNum.slice(1), fields: BUILD_FIELD_MASK }
      : { builder: { project, bucket, builder }, buildNumber: Number(buildIdOrNum), fields: BUILD_FIELD_MASK };
  } else {
    match = reqUrl.pathname.match(/^\/ui\/b\/(\d+)\/?/i);
    if (match) {
      req = { id: match[1], fields: BUILD_FIELD_MASK };
    }
  }

  if (!req) {
    return;
  }

  const authState = (await kvGet<AuthState | null>(AUTH_STATE_KEY)) || null;
  const prefetchBuildsService = new BuildsService(
    new PrpcClientExt(
      {
        host: CONFIGS.BUILDBUCKET.HOST,
        fetchImpl: async (info: RequestInfo, init?: RequestInit) => {
          const req = new Request(info, init);
          await cachedFetch(
            {},
            req,
            undefined,
            await genCacheKeyForPrpcRequest(PRPC_CACHE_KEY_PREFIX, req.clone()),
            // Because build info is time sensitive, don't keep it too long.
            5000
          );

          // Abort the function to prevent the response from being consumed.
          throw 0;
        },
      },
      () => authState?.accessToken || ''
    )
  );

  prefetchBuildsService
    // Bypass the service cache but trigger the cachedFetch cache.
    .getBuild(req, { acceptCache: false, skipUpdate: true })
    // Ignore any error, let the consumer of the cache deal with it.
    .catch((_e) => {});
}

/**
 * Prefetches the artifact if the url matches certain pattern.
 */
async function prefetchArtifact(reqUrl: URL) {
  const match = reqUrl.pathname.match(/^\/ui\/artifact\/([^/]+)\/([^/]+)\/?/i);
  if (!match) {
    return;
  }
  const artifactName = decodeURIComponent(match[2]);

  const authState = (await kvGet<AuthState | null>(AUTH_STATE_KEY)) || null;
  const prefetchResultDBService = new ResultDb(
    new PrpcClientExt(
      {
        host: CONFIGS.RESULT_DB.HOST,
        fetchImpl: async (info: RequestInfo, init?: RequestInit) => {
          const req = new Request(info, init);
          await cachedFetch(
            {},
            req,
            undefined,
            await genCacheKeyForPrpcRequest(PRPC_CACHE_KEY_PREFIX, req.clone()),
            // We could cache this until the fetchUrl expires, but that means
            // we need to clone the response. A short expire time should work
            // just fine as the browser should fetch and invalidate the cache
            // momentarily.
            5000
          );

          // Abort the function to prevent the response from being consumed.
          throw 0;
        },
      },
      () => authState?.accessToken || ''
    )
  );

  prefetchResultDBService
    // Bypass the service cache but trigger the cachedFetch cache.
    .getArtifact({ name: artifactName }, { acceptCache: false, skipUpdate: true })
    // Ignore any error, let the consumer of the cache deal with it.
    .catch((_e) => {});
}
