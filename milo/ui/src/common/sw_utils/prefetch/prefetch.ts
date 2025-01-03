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

import { BUILD_FIELD_MASK } from '@/build/constants';
import {
  getAuthStateCache,
  getAuthStateCacheSync,
  queryAuthState,
  setAuthStateCache,
} from '@/common/api/auth_state';
import {
  constructArtifactName,
  getInvIdFromBuildId,
  getInvIdFromBuildNum,
  RESULT_LIMIT,
  ResultDb,
} from '@/common/services/resultdb';
import { cached } from '@/generic_libs/tools/cached_fn';
import { PrpcClient } from '@/generic_libs/tools/prpc_client';
import { PrpcClientExt } from '@/generic_libs/tools/prpc_client_ext';
import { genCacheKeyForPrpcRequest } from '@/generic_libs/tools/prpc_utils';
import { timeout } from '@/generic_libs/tools/utils';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import {
  BuildsClientImpl,
  GetBuildRequest,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

const PRPC_CACHE_KEY_PREFIX = 'prpc-cache-key';
const AUTH_STATE_CACHE_KEY = Math.random().toString();

/**
 * Set the cache duration to 5s.
 * We don't want to cache the responses for too long because they can be
 * time-sensitive.
 *
 * We could cache some resources (e.g. finalized build, finalized invocation)
 * longer, but
 * 1. That requires more involved business logic, should be done at the
 * application layer rather than at the service worker layer.
 * 2. The browser should fetch and invalidate the cache momentarily anyway.
 * 3. If we don't invalidate the cache, we will need to clone the response,
 * which can be expensive when the response is only used once.
 */
const CACHE_DURATION = 5000;

// Cache option that can bypass the service cache but trigger the cachedFetch
// cache.
const CACHE_OPTION = { acceptCache: false, skipUpdate: true };

export class Prefetcher {
  private readonly authStateUrl = self.origin + '/auth/openid/state';
  private readonly cachedUrls: readonly string[];

  private cachedFetch = cached(
    // _cacheKey and _expiresIn are not used here but are used in the expire
    // and key functions below.
    // they are listed here to help TSC generate the correct type definition.
    (
      info: Parameters<typeof fetch>[0],
      init: Parameters<typeof fetch>[1],
      _cacheKey: unknown,
      _expiresIn: number,
    ) => this.fetchImpl(info, init),
    {
      key: (_info, _init, cacheKey) => cacheKey,
      expire: ([, , , expiresIn]) => timeout(expiresIn),
    },
  );

  private readonly prefetchBuildClient: BuildsClientImpl;
  private readonly prefetchResultDBService: ResultDb;

  constructor(
    private readonly settings: typeof SETTINGS,
    private readonly fetchImpl: typeof fetch,
  ) {
    this.cachedUrls = [
      this.authStateUrl,
      `https://${this.settings.buildbucket.host}/prpc/buildbucket.v2.Builds/GetBuild`,
      `https://${this.settings.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/GetArtifact`,
      `https://${this.settings.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/GetInvocation`,
      `https://${this.settings.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/QueryTestVariants`,
    ];
    this.prefetchBuildClient = new BuildsClientImpl(
      this.makeRpcClient(this.settings.buildbucket.host),
    );
    this.prefetchResultDBService = new ResultDb(
      this.makePrpcClient(this.settings.resultdb.host),
    );
  }

  /**
   * @deprecated use makeRpcClient instead.
   */
  private makePrpcClient(host: string) {
    return new PrpcClientExt(
      {
        host,
        fetchImpl: async (info, init?) => {
          const req = new Request(info, init);
          await this.cachedFetch(
            {},
            req,
            undefined,
            await genCacheKeyForPrpcRequest(PRPC_CACHE_KEY_PREFIX, req.clone()),
            CACHE_DURATION, // See the documentation for CACHE_DURATION.
          );

          // Abort the function to prevent the response from being consumed.
          throw new Error();
        },
      },

      () => getAuthStateCacheSync()?.accessToken || '',
    );
  }

  private makeRpcClient(host: string) {
    return new PrpcClient({
      host,
      getAuthToken: () => getAuthStateCacheSync()?.accessToken || '',
      fetchImpl: async (info, init?) => {
        const req = new Request(info, init);
        await this.cachedFetch(
          {},
          req,
          undefined,
          await genCacheKeyForPrpcRequest(PRPC_CACHE_KEY_PREFIX, req.clone()),
          CACHE_DURATION, // See the documentation for CACHE_DURATION.
        );

        // Abort the function to prevent the response from being consumed.
        throw new Error();
      },
    });
  }

  /**
   * Prefetches resources if the pathname matches certain pattern.
   * Those resources are cached for a short duration and are expected to be
   * fetched by the browser momentarily.
   */
  async prefetchResources(pathname: string) {
    // Prefetch services relies on the in-memory cache.
    // Call getAuthState to populate the in-memory cache.
    const authState = await getAuthStateCache();

    const queryAuthStatePromise = queryAuthState((info, init) =>
      this.cachedFetch(
        {},
        info,
        init,
        AUTH_STATE_CACHE_KEY,
        CACHE_DURATION,
      ).then((res) => res.clone()),
    ).then(setAuthStateCache);
    if (!authState) {
      await queryAuthStatePromise;
    }

    this.prefetchBuildPageResources(pathname);
    this.prefetchArtifactPageResources(pathname);
  }

  /**
   * Prefetches build page related resources if the URL matches certain pattern.
   */
  private async prefetchBuildPageResources(pathname: string) {
    let buildId: string | null = null;
    let buildNum: number | null = null;
    let builderId: BuilderID | null = null;
    let invName: string | null = null;

    let match = pathname.match(
      /^\/ui\/p\/([^/]+)\/builders\/([^/]+)\/([^/]+)\/(b?\d+)\/?/i,
    );
    if (match) {
      const [project, bucket, builder, buildIdOrNum] = match
        .slice(1, 5)
        .map((v) => decodeURIComponent(v));
      if (buildIdOrNum.startsWith('b')) {
        buildId = buildIdOrNum.slice(1);
      } else {
        buildNum = Number(buildIdOrNum);
        builderId = { project, bucket, builder };
      }
    } else {
      match = pathname.match(/^\/ui\/b\/(\d+)\/?/i);
      if (match) {
        buildId = match[1];
      }
    }

    let getBuildRequest: GetBuildRequest | null = null;
    if (buildId) {
      getBuildRequest = GetBuildRequest.fromPartial({
        id: buildId,
        mask: {
          fields: BUILD_FIELD_MASK,
        },
      });
      invName = 'invocations/' + getInvIdFromBuildId(buildId);
    } else if (builderId && buildNum) {
      getBuildRequest = GetBuildRequest.fromPartial({
        builder: builderId,
        buildNumber: buildNum,
        mask: {
          fields: BUILD_FIELD_MASK,
        },
      });
      invName =
        'invocations/' + (await getInvIdFromBuildNum(builderId, buildNum));
    }

    if (getBuildRequest) {
      this.prefetchBuildClient.GetBuild(getBuildRequest).catch((_e) => {
        // Ignore any error, let the consumer of the cache deal with it.
      });
    }

    if (invName) {
      this.prefetchResultDBService
        .getInvocation({ name: invName }, CACHE_OPTION)
        .catch((_e) => {
          // Ignore any error, let the consumer of the cache deal with it.
        });
      this.prefetchResultDBService
        .queryTestVariants(
          { invocations: [invName], resultLimit: RESULT_LIMIT },
          CACHE_OPTION,
        )
        .catch((_e) => {
          // Ignore any error, let the consumer of the cache deal with it.
        });
    }
  }

  /**
   * Prefetches artifact page related resources if the URL matches certain
   * pattern.
   */
  private async prefetchArtifactPageResources(pathname: string) {
    const match = pathname.match(
      /^\/ui\/artifact\/(?:[^/]+)\/invocations\/([^/]+)(?:\/tests\/([^/]+)\/results\/([^/]+))?\/artifacts\/([^/]+)\/?/i,
    );
    if (!match) {
      return;
    }
    const [invocationId, testId, resultId, artifactId] = match
      .slice(1, 5)
      .map((v) => (v === undefined ? undefined : decodeURIComponent(v)));

    this.prefetchResultDBService
      .getArtifact(
        {
          name: constructArtifactName({
            // Invocation is a compulsory capture group.
            // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
            invocationId: invocationId!,
            testId,
            resultId,
            // artifactId is a compulsory capture group.
            // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
            artifactId: artifactId!,
          }),
        },
        CACHE_OPTION,
      )
      .catch((_e) => {
        // Ignore any error, let the consumer of the cache deal with it.
      });
  }

  /**
   * Responds to the event with the cached content if the URL matches certain
   * pattern.
   *
   * Returns true if the URL might be cached. Returns false otherwise.
   */
  respondWithPrefetched(e: FetchEvent) {
    if (!this.cachedUrls.includes(e.request.url)) {
      return false;
    }

    e.respondWith(
      (async () => {
        const cacheKey =
          e.request.url === this.authStateUrl
            ? AUTH_STATE_CACHE_KEY
            : await genCacheKeyForPrpcRequest(
                PRPC_CACHE_KEY_PREFIX,
                e.request.clone(),
              );

        const res = await this.cachedFetch(
          // The response can't be reused, don't keep it in cache.
          { skipUpdate: true, invalidateCache: true },
          e.request,
          undefined,
          cacheKey,
          0,
        );
        return res;
      })(),
    );

    return true;
  }
}
