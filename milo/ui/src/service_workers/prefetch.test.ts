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

import { beforeEach, expect, jest } from '@jest/globals';
import { aTimeout } from '@open-wc/testing-helpers';

import { queryAuthState, setAuthStateCache } from '../libs/auth_state';
import { PrpcClientExt } from '../libs/prpc_client_ext';
import { BUILD_FIELD_MASK, BuildsService } from '../services/buildbucket';
import {
  getInvIdFromBuildId,
  getInvIdFromBuildNum,
  RESULT_LIMIT,
  ResultDb,
} from '../services/resultdb';
import { Prefetcher } from './prefetch';

describe('Prefetcher', () => {
  let fetchStub: jest.Mock<{
    (
      input: RequestInfo | URL,
      init?: RequestInit | undefined
    ): Promise<Response>;
  }>;
  let respondWithStub: jest.Mock<
    (_res: Response | ReturnType<typeof fetch>) => void
  >;
  let prefetcher: Prefetcher;

  // Helps generate fetch requests that are identical to the ones generated
  // by the pRPC Clients.
  let fetchInterceptor: jest.Mock<{
    (
      input: RequestInfo | URL,
      init?: RequestInit | undefined
    ): Promise<Response>;
  }>;
  let buildsService: BuildsService;
  let resultdb: ResultDb;

  beforeEach(async () => {
    await setAuthStateCache({
      accessToken: 'access-token',
      identity: 'user:user-id',
    });

    fetchStub = jest.fn(fetch);
    respondWithStub = jest.fn(
      (_res: Response | ReturnType<typeof fetch>) => {}
    );
    prefetcher = new Prefetcher(CONFIGS, fetchStub);

    fetchInterceptor = jest.fn(fetch);
    fetchInterceptor.mockResolvedValue(new Response(''));
    buildsService = new BuildsService(
      new PrpcClientExt(
        { host: CONFIGS.BUILDBUCKET.HOST, fetchImpl: fetchInterceptor },
        () => 'access-token'
      )
    );
    resultdb = new ResultDb(
      new PrpcClientExt(
        { host: CONFIGS.RESULT_DB.HOST, fetchImpl: fetchInterceptor },
        () => 'access-token'
      )
    );
  });

  it('prefetches build page resources', async () => {
    const authResponse = new Response(
      JSON.stringify({ accessToken: 'access-token', identity: 'user:user-id' })
    );
    const buildResponse = new Response(JSON.stringify({}));
    const invResponse = new Response(JSON.stringify({}));
    const testVariantsResponse = new Response(JSON.stringify({}));

    fetchStub.mockResolvedValueOnce(authResponse);
    fetchStub.mockResolvedValueOnce(buildResponse);
    fetchStub.mockResolvedValueOnce(invResponse);
    fetchStub.mockResolvedValueOnce(testVariantsResponse);

    const invName =
      'invocations/' +
      (await getInvIdFromBuildNum(
        {
          project: 'chromium',
          bucket: 'ci',
          builder: 'Win7 Tests (1)',
        },
        116372
      ));

    await prefetcher.prefetchResources(
      new URL(
        'https://luci-milo-dev.appspot.com/ui/p/chromium/builders/ci/Win7%20Tests%20(1)/116372'
      )
    );

    await aTimeout(100);

    const requestedUrls = fetchStub.mock.calls.map(
      (c) => new Request(...c).url
    );
    expect(requestedUrls.length).toStrictEqual(4);
    expect(requestedUrls).toEqual(
      expect.arrayContaining([
        '/auth/openid/state',
        `https://${CONFIGS.BUILDBUCKET.HOST}/prpc/buildbucket.v2.Builds/GetBuild`,
        `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/GetInvocation`,
        `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/QueryTestVariants`,
      ])
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
    expect(fetchStub.mock.calls.length).toStrictEqual(4);

    // Generate a fetch request.
    buildsService
      .getBuild({
        builder: {
          project: 'chromium',
          bucket: 'ci',
          builder: 'Win7 Tests (1)',
        },
        buildNumber: 116372,
        fields: BUILD_FIELD_MASK,
      })
      .catch((_e) => {});
    // Check whether the build was prefetched.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[1]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.mock.calls[1][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(buildResponse);
    expect(fetchStub.mock.calls.length).toStrictEqual(4);

    // Generate a fetch request.
    resultdb.getInvocation({ name: invName }).catch((_e) => {});
    // Check whether the invocation was prefetched.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[2]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.mock.calls[2][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(invResponse);
    expect(fetchStub.mock.calls.length).toStrictEqual(4);

    // Generate a fetch request.
    resultdb
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
    expect(fetchStub.mock.calls.length).toStrictEqual(4);
  });

  it('prefetches build page resources when visiting a short build page url', async () => {
    const authResponse = new Response(
      JSON.stringify({ accessToken: 'access-token', identity: 'user:user-id' })
    );
    const buildResponse = new Response(JSON.stringify({}));
    const invResponse = new Response(JSON.stringify({}));
    const testVariantsResponse = new Response(JSON.stringify({}));

    fetchStub.mockResolvedValueOnce(authResponse);
    fetchStub.mockResolvedValueOnce(buildResponse);
    fetchStub.mockResolvedValueOnce(invResponse);
    fetchStub.mockResolvedValueOnce(testVariantsResponse);

    const invName = 'invocations/' + getInvIdFromBuildId('123456789');

    await prefetcher.prefetchResources(
      new URL('https://luci-milo-dev.appspot.com/ui/b/123456789')
    );

    await aTimeout(100);

    const requestedUrls = fetchStub.mock.calls.map(
      (c) => new Request(...c).url
    );
    expect(requestedUrls.length).toStrictEqual(4);
    expect(requestedUrls).toEqual(
      expect.arrayContaining([
        '/auth/openid/state',
        `https://${CONFIGS.BUILDBUCKET.HOST}/prpc/buildbucket.v2.Builds/GetBuild`,
        `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/GetInvocation`,
        `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/QueryTestVariants`,
      ])
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
    expect(fetchStub.mock.calls.length).toStrictEqual(4);

    // Generate a fetch request.
    buildsService
      .getBuild({
        id: '123456789',
        fields: BUILD_FIELD_MASK,
      })
      .catch((_e) => {});
    // Check whether the build was prefetched.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[1]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.mock.calls[1][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(buildResponse);
    expect(fetchStub.mock.calls.length).toStrictEqual(4);

    // Generate a fetch request.
    resultdb.getInvocation({ name: invName }).catch((_e) => {});
    // Check whether the invocation was prefetched.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.mock.calls[2]),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.mock.calls[2][0];

    expect(cacheHit).toBeTruthy();
    expect(cachedRes).toStrictEqual(invResponse);
    expect(fetchStub.mock.calls.length).toStrictEqual(4);

    // Generate a fetch request.
    resultdb
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
    expect(fetchStub.mock.calls.length).toStrictEqual(4);
  });

  it('prefetches artifact page resources', async () => {
    const authResponse = new Response(
      JSON.stringify({ accessToken: 'access-token', identity: 'user:user-id' })
    );
    const artifactResponse = new Response(
      JSON.stringify({
        name: 'invocations/inv-id/tests/test-id/results/result-id/artifacts/artifact-id',
        artifactId: 'artifact-id',
      })
    );

    fetchStub.mockResolvedValueOnce(authResponse);
    fetchStub.mockResolvedValueOnce(artifactResponse);

    await prefetcher.prefetchResources(
      new URL(
        // eslint-disable-next-line max-len
        'https://luci-milo-dev.appspot.com/ui/artifact/raw/invocations/inv-id/tests/test-id/results/result-id/artifacts/artifact-id'
      )
    );

    await aTimeout(100);

    const requestedUrls = fetchStub.mock.calls.map(
      (c) => new Request(...c).url
    );
    expect(requestedUrls.length).toStrictEqual(2);
    expect(requestedUrls).toEqual(
      expect.arrayContaining([
        '/auth/openid/state',
        `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/GetArtifact`,
      ])
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
    expect(fetchStub.mock.calls.length).toStrictEqual(2);

    // Generate a fetch request.
    resultdb
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
    expect(fetchStub.mock.calls.length).toStrictEqual(2);
    expect(cachedRes).toStrictEqual(artifactResponse);
  });
});
