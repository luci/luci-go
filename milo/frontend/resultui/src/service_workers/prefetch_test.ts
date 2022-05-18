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

import { aTimeout } from '@open-wc/testing/index-no-side-effects';
import { assert } from 'chai';
import * as sinon from 'sinon';

import { setAuthStateCache } from '../auth_state_cache';
import { PrpcClientExt } from '../libs/prpc_client_ext';
import { BUILD_FIELD_MASK, BuildsService } from '../services/buildbucket';
import { queryAuthState } from '../services/milo_internal';
import { getInvIdFromBuildId, getInvIdFromBuildNum, ResultDb } from '../services/resultdb';
import { Prefetcher } from './prefetch';

describe('prefetch', () => {
  let fetchStub: sinon.SinonStub<Parameters<typeof fetch>, ReturnType<typeof fetch>>;
  let respondWithStub: sinon.SinonStub<[Response | ReturnType<typeof fetch>], void>;
  let prefetcher: Prefetcher;

  // Helps generate fetch requests that are identical to the ones generated
  // by the pRPC Clients.
  let fetchInterceptor: sinon.SinonStub<Parameters<typeof fetch>, ReturnType<typeof fetch>>;
  let buildsService: BuildsService;
  let resultdb: ResultDb;

  beforeEach(async () => {
    await setAuthStateCache({ accessToken: 'access-token', identity: 'user:user-id' });

    fetchStub = sinon.stub<Parameters<typeof fetch>, ReturnType<typeof fetch>>();
    respondWithStub = sinon.stub<[Response | ReturnType<typeof fetch>], void>();
    prefetcher = new Prefetcher(CONFIGS, fetchStub);

    fetchInterceptor = sinon.stub<Parameters<typeof fetch>, ReturnType<typeof fetch>>();
    fetchInterceptor.resolves(new Response(''));
    buildsService = new BuildsService(
      new PrpcClientExt({ host: CONFIGS.BUILDBUCKET.HOST, fetchImpl: fetchInterceptor }, () => 'access-token')
    );
    resultdb = new ResultDb(
      new PrpcClientExt({ host: CONFIGS.RESULT_DB.HOST, fetchImpl: fetchInterceptor }, () => 'access-token')
    );
  });

  it('prefetches build page resources', async () => {
    const authResponse = new Response(JSON.stringify({}));
    const buildResponse = new Response(JSON.stringify({}));
    const invResponse = new Response(JSON.stringify({}));
    const testVariantsResponse = new Response(JSON.stringify({}));

    fetchStub.onCall(0).resolves(authResponse);
    fetchStub.onCall(1).resolves(buildResponse);
    fetchStub.onCall(2).resolves(invResponse);
    fetchStub.onCall(3).resolves(testVariantsResponse);

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
      new URL('https://luci-milo-dev.appspot.com/ui/p/chromium/builders/ci/Win7%20Tests%20(1)/116372')
    );

    await aTimeout(100);

    const requestedUrls = fetchStub.getCalls().map((c) => new Request(...c.args).url);
    assert.strictEqual(requestedUrls.length, 4);
    assert.includeMembers(requestedUrls, [
      `http://${self.location.host}/auth-state`,
      `https://${CONFIGS.BUILDBUCKET.HOST}/prpc/buildbucket.v2.Builds/GetBuild`,
      `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/GetInvocation`,
      `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/QueryTestVariants`,
    ]);

    // Check whether the auth state was prefetched.
    await queryAuthState(fetchInterceptor).catch((_e) => {});
    let cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(0).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    let cachedRes = await respondWithStub.getCall(0).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, authResponse);
    assert.strictEqual(fetchStub.callCount, 4);

    // Check whether the build was prefetched.
    buildsService.getBuild({
      builder: {
        project: 'chromium',
        bucket: 'ci',
        builder: 'Win7 Tests (1)',
      },
      buildNumber: 116372,
      fields: BUILD_FIELD_MASK,
    });
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(1).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.getCall(1).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, buildResponse);
    assert.strictEqual(fetchStub.callCount, 4);

    // Check whether the invocation was prefetched.
    resultdb.getInvocation({ name: invName });
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(2).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.getCall(2).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, invResponse);
    assert.strictEqual(fetchStub.callCount, 4);

    // Check whether the test variants was prefetched.
    resultdb.queryTestVariants({ invocations: [invName] });
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(3).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.getCall(3).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, testVariantsResponse);
    assert.strictEqual(fetchStub.callCount, 4);
  });

  it('prefetches build page resources when visiting a short build page url', async () => {
    const authResponse = new Response(JSON.stringify({}));
    const buildResponse = new Response(JSON.stringify({}));
    const invResponse = new Response(JSON.stringify({}));
    const testVariantsResponse = new Response(JSON.stringify({}));

    fetchStub.onCall(0).resolves(authResponse);
    fetchStub.onCall(1).resolves(buildResponse);
    fetchStub.onCall(2).resolves(invResponse);
    fetchStub.onCall(3).resolves(testVariantsResponse);

    const invName = 'invocations/' + getInvIdFromBuildId('123456789');

    await prefetcher.prefetchResources(new URL('https://luci-milo-dev.appspot.com/ui/b/123456789'));

    await aTimeout(100);

    const requestedUrls = fetchStub.getCalls().map((c) => new Request(...c.args).url);
    assert.strictEqual(requestedUrls.length, 4);
    assert.includeMembers(requestedUrls, [
      `http://${self.location.host}/auth-state`,
      `https://${CONFIGS.BUILDBUCKET.HOST}/prpc/buildbucket.v2.Builds/GetBuild`,
      `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/GetInvocation`,
      `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/QueryTestVariants`,
    ]);

    // Check whether the auth state was prefetched.
    await queryAuthState(fetchInterceptor).catch((_e) => {});
    let cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(0).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    let cachedRes = await respondWithStub.getCall(0).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, authResponse);
    assert.strictEqual(fetchStub.callCount, 4);

    // Check whether the build was prefetched.
    buildsService.getBuild({
      id: '123456789',
      fields: BUILD_FIELD_MASK,
    });
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(1).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.getCall(1).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, buildResponse);
    assert.strictEqual(fetchStub.callCount, 4);

    // Check whether the invocation was prefetched.
    resultdb.getInvocation({ name: invName });
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(2).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.getCall(2).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, invResponse);
    assert.strictEqual(fetchStub.callCount, 4);

    // Check whether the test variants was prefetched.
    resultdb.queryTestVariants({ invocations: [invName] });
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(3).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.getCall(3).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, testVariantsResponse);
    assert.strictEqual(fetchStub.callCount, 4);
  });

  it('prefetches artifact page resources', async () => {
    const authResponse = new Response(JSON.stringify({}));
    const artifactResponse = new Response(
      JSON.stringify({
        name: 'invocations/inv-id/tests/test-id/results/result-id/artifacts/artifact-id',
        artifactId: 'artifact-id',
      })
    );

    fetchStub.onCall(0).resolves(authResponse);
    fetchStub.onCall(1).resolves(artifactResponse);

    await prefetcher.prefetchResources(
      new URL(
        // eslint-disable-next-line max-len
        'https://luci-milo-dev.appspot.com/ui/artifact/raw/invocations/inv-id/tests/test-id/results/result-id/artifacts/artifact-id'
      )
    );

    await aTimeout(100);

    const requestedUrls = fetchStub.getCalls().map((c) => new Request(...c.args).url);
    assert.strictEqual(requestedUrls.length, 2);
    assert.includeMembers(requestedUrls, [
      `http://${self.location.host}/auth-state`,
      `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/GetArtifact`,
    ]);

    // Check whether the auth state was prefetched.
    await queryAuthState(fetchInterceptor).catch((_e) => {});
    let cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(0).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    let cachedRes = await respondWithStub.getCall(0).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, authResponse);
    assert.strictEqual(fetchStub.callCount, 2);

    // Check whether the artifact was prefetched.
    resultdb.getArtifact({
      name: 'invocations/inv-id/tests/test-id/results/result-id/artifacts/artifact-id',
    });
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(1).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.getCall(1).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(fetchStub.callCount, 2);
    assert.strictEqual(cachedRes, artifactResponse);
  });
});
