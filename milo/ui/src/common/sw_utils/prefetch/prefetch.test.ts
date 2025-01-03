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
import { queryAuthState, setAuthStateCache } from '@/common/api/auth_state';
import {
  getInvIdFromBuildId,
  getInvIdFromBuildNum,
  RESULT_LIMIT,
  ResultDb,
} from '@/common/services/resultdb';
import { PrpcClient } from '@/generic_libs/tools/prpc_client';
import { PrpcClientExt } from '@/generic_libs/tools/prpc_client_ext';
import {
  BuildsClientImpl,
  GetBuildRequest,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

import { Prefetcher } from './prefetch';

describe('Prefetcher', () => {
  let fetchStub: jest.MockedFunction<typeof fetch>;
  let respondWithStub: jest.MockedFunction<
    (_res: Response | ReturnType<typeof fetch>) => void
  >;
  let prefetcher: Prefetcher;

  // Helps generate fetch requests that are identical to the ones generated
  // by the pRPC Clients.
  let fetchInterceptor: jest.MockedFunction<typeof fetch>;
  let buildsClient: BuildsClientImpl;
  let resultdb: ResultDb;

  beforeEach(async () => {
    jest.useFakeTimers();
    await setAuthStateCache({
      accessToken: 'access-token',
      identity: 'user:user-id',
    });

    fetchStub = jest.fn(fetch);
    respondWithStub = jest.fn(
      (_res: Response | ReturnType<typeof fetch>) => {},
    );
    prefetcher = new Prefetcher(SETTINGS, fetchStub);

    fetchInterceptor = jest.fn(fetch);
    fetchInterceptor.mockResolvedValue(new Response(''));
    buildsClient = new BuildsClientImpl(
      new PrpcClient({
        host: SETTINGS.buildbucket.host,
        getAuthToken: () => 'access-token',
        fetchImpl: fetchInterceptor,
      }),
    );
    resultdb = new ResultDb(
      new PrpcClientExt(
        { host: SETTINGS.resultdb.host, fetchImpl: fetchInterceptor },
        () => 'access-token',
      ),
    );
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('prefetches build page resources', async () => {
    const authResponse = new Response(
      JSON.stringify({ accessToken: 'access-token', identity: 'user:user-id' }),
    );
    const buildResponse = new Response(JSON.stringify({}));
    const invResponse = new Response(JSON.stringify({}));
    const testVariantsResponse = new Response(JSON.stringify({}));

    fetchStub.mockImplementation(async (input, init) => {
      const req = new Request(input, init);
      switch (req.url) {
        case self.origin + '/auth/openid/state':
          return authResponse;
        case `https://${SETTINGS.buildbucket.host}/prpc/buildbucket.v2.Builds/GetBuild`:
          return buildResponse;
        case `https://${SETTINGS.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/GetInvocation`:
          return invResponse;
        case `https://${SETTINGS.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/QueryTestVariants`:
          return testVariantsResponse;
        default:
          throw new Error('unexpected request URL');
      }
    });

    const invName =
      'invocations/' +
      (await getInvIdFromBuildNum(
        {
          project: 'chromium',
          bucket: 'ci',
          builder: 'Win7 Tests (1)',
        },
        116372,
      ));

    await prefetcher.prefetchResources(
      '/ui/p/chromium/builders/ci/Win7%20Tests%20(1)/116372',
    );

    await jest.advanceTimersByTimeAsync(100);
    await jest.advanceTimersByTimeAsync(100);

    const requestedUrls = fetchStub.mock.calls.map(
      (c) => new Request(...c).url,
    );
    expect(requestedUrls.length).toStrictEqual(4);
    expect(requestedUrls).toEqual(
      expect.arrayContaining([
        self.origin + '/auth/openid/state',
        `https://${SETTINGS.buildbucket.host}/prpc/buildbucket.v2.Builds/GetBuild`,
        `https://${SETTINGS.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/GetInvocation`,
        `https://${SETTINGS.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/QueryTestVariants`,
      ]),
    );

    // Generate a fetch request.
    await queryAuthState(fetchInterceptor).catch((_e) => {});
    let cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[0]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    // Check whether the auth state was prefetched.
    expect(cacheHit).toBeTruthy();
    let cachedRes = await respondWithStub.mock.calls[0][0];

    expect(cachedRes).toStrictEqual(authResponse);
    expect(fetchStub).toHaveBeenCalledTimes(4);

    // Generate a fetch request.
    await buildsClient
      .GetBuild(
        GetBuildRequest.fromPartial({
          builder: {
            project: 'chromium',
            bucket: 'ci',
            builder: 'Win7 Tests (1)',
          },
          buildNumber: 116372,
          mask: {
            fields: BUILD_FIELD_MASK,
          },
        }),
      )
      .catch((_e) => {});
    // Check whether the build was prefetched.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[1]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.mock.calls[1][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(buildResponse);
    expect(fetchStub).toHaveBeenCalledTimes(4);

    // Generate a fetch request.
    await resultdb.getInvocation({ name: invName }).catch((_e) => {});
    // Check whether the invocation was prefetched.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[2]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.mock.calls[2][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(invResponse);
    expect(fetchStub).toHaveBeenCalledTimes(4);

    // Generate a fetch request.
    await resultdb
      .queryTestVariants({ invocations: [invName], resultLimit: RESULT_LIMIT })
      .catch((_e) => {});
    // Check whether the test variants was prefetched.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[3]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.mock.calls[3][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(testVariantsResponse);
    expect(fetchStub).toHaveBeenCalledTimes(4);
  });

  it('prefetches build page resources when visiting a short build page url', async () => {
    const authResponse = new Response(
      JSON.stringify({ accessToken: 'access-token', identity: 'user:user-id' }),
    );
    const buildResponse = new Response(JSON.stringify({}));
    const invResponse = new Response(JSON.stringify({}));
    const testVariantsResponse = new Response(JSON.stringify({}));

    fetchStub.mockImplementation(async (input, init) => {
      const req = new Request(input, init);
      switch (req.url) {
        case self.origin + '/auth/openid/state':
          return authResponse;
        case `https://${SETTINGS.buildbucket.host}/prpc/buildbucket.v2.Builds/GetBuild`:
          return buildResponse;
        case `https://${SETTINGS.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/GetInvocation`:
          return invResponse;
        case `https://${SETTINGS.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/QueryTestVariants`:
          return testVariantsResponse;
        default:
          throw new Error('unexpected request URL');
      }
    });

    const invName = 'invocations/' + getInvIdFromBuildId('123456789');

    await prefetcher.prefetchResources('/ui/b/123456789');

    await jest.advanceTimersByTimeAsync(100);

    const requestedUrls = fetchStub.mock.calls.map(
      (c) => new Request(...c).url,
    );
    expect(requestedUrls.length).toStrictEqual(4);
    expect(requestedUrls).toEqual(
      expect.arrayContaining([
        self.origin + '/auth/openid/state',
        `https://${SETTINGS.buildbucket.host}/prpc/buildbucket.v2.Builds/GetBuild`,
        `https://${SETTINGS.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/GetInvocation`,
        `https://${SETTINGS.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/QueryTestVariants`,
      ]),
    );

    // Generate a fetch request.
    await queryAuthState(fetchInterceptor).catch((_e) => {});
    // Check whether the auth state was prefetched.
    let cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[0]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    let cachedRes = await respondWithStub.mock.calls[0][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(authResponse);
    expect(fetchStub).toHaveBeenCalledTimes(4);

    // Generate a fetch request.
    await buildsClient
      .GetBuild(
        GetBuildRequest.fromPartial({
          id: '123456789',
          mask: {
            fields: BUILD_FIELD_MASK,
          },
        }),
      )
      .catch((_e) => {});
    // Check whether the build was prefetched.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[1]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.mock.calls[1][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(buildResponse);
    expect(fetchStub).toHaveBeenCalledTimes(4);

    // Generate a fetch request.
    await resultdb.getInvocation({ name: invName }).catch((_e) => {});
    // Check whether the invocation was prefetched.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[2]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.mock.calls[2][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(invResponse);
    expect(fetchStub).toHaveBeenCalledTimes(4);

    // Generate a fetch request.
    await resultdb
      .queryTestVariants({ invocations: [invName], resultLimit: RESULT_LIMIT })
      .catch((_e) => {});
    // Check whether the test variants was prefetched.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[3]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.mock.calls[3][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(testVariantsResponse);
    expect(fetchStub).toHaveBeenCalledTimes(4);
  });

  it('prefetches artifact page resources', async () => {
    const authResponse = new Response(
      JSON.stringify({ accessToken: 'access-token', identity: 'user:user-id' }),
    );
    const artifactResponse = new Response(
      JSON.stringify({
        name: 'invocations/inv-id/tests/test-id/results/result-id/artifacts/artifact-id',
        artifactId: 'artifact-id',
      }),
    );

    fetchStub.mockImplementation(async (input, init) => {
      const req = new Request(input, init);
      switch (req.url) {
        case self.origin + '/auth/openid/state':
          return authResponse;
        case `https://${SETTINGS.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/GetArtifact`:
          return artifactResponse;
        default:
          throw new Error('unexpected request URL');
      }
    });

    await prefetcher.prefetchResources(
      '/ui/artifact/raw/invocations/inv-id/tests/test-id/results/result-id/artifacts/artifact-id',
    );

    await jest.advanceTimersByTimeAsync(100);

    const requestedUrls = fetchStub.mock.calls.map(
      (c) => new Request(...c).url,
    );
    expect(requestedUrls.length).toStrictEqual(2);
    expect(requestedUrls).toEqual(
      expect.arrayContaining([
        self.origin + '/auth/openid/state',
        `https://${SETTINGS.resultdb.host}/prpc/luci.resultdb.v1.ResultDB/GetArtifact`,
      ]),
    );

    // Generate a fetch request.
    await queryAuthState(fetchInterceptor).catch((_e) => {});
    // Check whether the auth state was prefetched.
    let cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[0]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    let cachedRes = await respondWithStub.mock.calls[0][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(authResponse);
    expect(fetchStub).toHaveBeenCalledTimes(2);

    // Generate a fetch request.
    await resultdb
      .getArtifact({
        name: 'invocations/inv-id/tests/test-id/results/result-id/artifacts/artifact-id',
      })
      .catch((_e) => {});
    // Check whether the artifact was prefetched.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[1]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.mock.calls[1][0];

    expect(cacheHit).toBeTruthy();
    expect(fetchStub).toHaveBeenCalledTimes(2);
    expect(cachedRes).toStrictEqual(artifactResponse);
  });
});
