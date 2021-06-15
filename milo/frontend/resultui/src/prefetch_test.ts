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

import { setAuthState } from './auth_state';
import { PrpcClientExt } from './libs/prpc_client_ext';
import { Prefetcher } from './prefetch';
import { BUILD_FIELD_MASK, BuildsService } from './services/buildbucket';
import { getInvIdFromBuildId, getInvIdFromBuildNum, ResultDb, UISpecificService } from './services/resultdb';

describe('prefetch', () => {
  let fetchStub: sinon.SinonStub<[RequestInfo, RequestInit | undefined], Promise<Response>>;
  let respondWithStub: sinon.SinonStub<[Response | Promise<Response>], void>;
  let prefetcher: Prefetcher;

  // Helps generate fetch requests that are identical to the ones generated
  // by the pRPC Clients.
  let fetchInterceptor: sinon.SinonStub<[RequestInfo, RequestInit | undefined], Promise<Response>>;
  let buildsService: BuildsService;
  let resultdb: ResultDb;
  let uiSpecifiedService: UISpecificService;

  beforeEach(async () => {
    await setAuthState({ accessToken: 'access-token', identity: 'user:user-id' });

    fetchStub = sinon.stub<[RequestInfo, RequestInit | undefined], Promise<Response>>();
    respondWithStub = sinon.stub<[Response | Promise<Response>], void>();
    prefetcher = new Prefetcher(CONFIGS, fetchStub);

    fetchInterceptor = sinon.stub<[RequestInfo, RequestInit | undefined], Promise<Response>>();
    buildsService = new BuildsService(
      new PrpcClientExt({ host: CONFIGS.BUILDBUCKET.HOST, fetchImpl: fetchInterceptor }, () => 'access-token')
    );
    resultdb = new ResultDb(
      new PrpcClientExt({ host: CONFIGS.RESULT_DB.HOST, fetchImpl: fetchInterceptor }, () => 'access-token')
    );
    uiSpecifiedService = new UISpecificService(
      new PrpcClientExt({ host: CONFIGS.RESULT_DB.HOST, fetchImpl: fetchInterceptor }, () => 'access-token')
    );
  });

  it('prefetches build page resources', async () => {
    const buildResponse = new Response(JSON.stringify({}));
    const invResponse = new Response(JSON.stringify({}));
    const testVariantsResponse = new Response(JSON.stringify({}));

    fetchStub.onCall(0).resolves(buildResponse);
    fetchStub.onCall(1).resolves(invResponse);
    fetchStub.onCall(2).resolves(testVariantsResponse);

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
    assert.strictEqual(requestedUrls.length, 3);
    assert.includeMembers(requestedUrls, [
      `https://${CONFIGS.BUILDBUCKET.HOST}/prpc/buildbucket.v2.Builds/GetBuild`,
      `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/GetInvocation`,
      `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.internal.ui.UI/QueryTestVariants`,
    ]);

    buildsService.getBuild({
      builder: {
        project: 'chromium',
        bucket: 'ci',
        builder: 'Win7 Tests (1)',
      },
      buildNumber: 116372,
      fields: BUILD_FIELD_MASK,
    });

    // Prefetched build.
    let cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(0).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    let cachedRes = await respondWithStub.getCall(0).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, buildResponse);
    assert.strictEqual(fetchStub.callCount, 3);

    resultdb.getInvocation({ name: invName });

    // Prefetched invocation.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(1).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.getCall(1).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, invResponse);
    assert.strictEqual(fetchStub.callCount, 3);

    uiSpecifiedService.queryTestVariants({ invocations: [invName] });

    // Prefetched test variants.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(2).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.getCall(2).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, testVariantsResponse);
    assert.strictEqual(fetchStub.callCount, 3);
  });

  it('prefetches build page resources when visiting a short build page url', async () => {
    const buildResponse = new Response(JSON.stringify({}));
    const invResponse = new Response(JSON.stringify({}));
    const testVariantsResponse = new Response(JSON.stringify({}));

    fetchStub.onCall(0).resolves(buildResponse);
    fetchStub.onCall(1).resolves(invResponse);
    fetchStub.onCall(2).resolves(testVariantsResponse);

    const invName = 'invocations/' + getInvIdFromBuildId('123456789');

    await prefetcher.prefetchResources(new URL('https://luci-milo-dev.appspot.com/ui/b/123456789'));

    await aTimeout(100);

    const requestedUrls = fetchStub.getCalls().map((c) => new Request(...c.args).url);
    assert.strictEqual(requestedUrls.length, 3);
    assert.includeMembers(requestedUrls, [
      `https://${CONFIGS.BUILDBUCKET.HOST}/prpc/buildbucket.v2.Builds/GetBuild`,
      `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/GetInvocation`,
      `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.internal.ui.UI/QueryTestVariants`,
    ]);

    buildsService.getBuild({
      id: '123456789',
      fields: BUILD_FIELD_MASK,
    });

    // Prefetched build.
    let cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(0).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    let cachedRes = await respondWithStub.getCall(0).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, buildResponse);
    assert.strictEqual(fetchStub.callCount, 3);

    resultdb.getInvocation({ name: invName });

    // Prefetched invocation.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(1).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.getCall(1).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, invResponse);
    assert.strictEqual(fetchStub.callCount, 3);

    uiSpecifiedService.queryTestVariants({ invocations: [invName] });

    // Prefetched test variants.
    cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(2).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    cachedRes = await respondWithStub.getCall(2).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(cachedRes, testVariantsResponse);
    assert.strictEqual(fetchStub.callCount, 3);
  });

  it('prefetches artifact page resources', async () => {
    const artifactResponse = new Response(
      JSON.stringify({
        name: 'invocations/inv-id/tests/test-id/results/result-id/artifacts/artifact-id',
        artifactId: 'artifact-id',
      })
    );
    fetchStub.onCall(0).resolves(artifactResponse);

    await prefetcher.prefetchResources(
      new URL(
        // eslint-disable-next-line max-len
        'https://luci-milo-dev.appspot.com/ui/artifact/raw/invocations/inv-id/tests/test-id/results/result-id/artifacts/artifact-id'
      )
    );

    await aTimeout(100);

    const requestedUrls = fetchStub.getCalls().map((c) => new Request(...c.args).url);
    assert.strictEqual(requestedUrls.length, 1);
    assert.includeMembers(requestedUrls, [
      `https://${CONFIGS.RESULT_DB.HOST}/prpc/luci.resultdb.v1.ResultDB/GetArtifact`,
    ]);

    resultdb.getArtifact({
      name: 'invocations/inv-id/tests/test-id/results/result-id/artifacts/artifact-id',
    });

    const cacheHit = prefetcher.respondWithPrefetched({
      request: new Request(...fetchInterceptor.getCall(0).args),
      respondWith: respondWithStub,
    } as Partial<FetchEvent> as FetchEvent);
    const cachedRes = await respondWithStub.getCall(0).args[0];

    assert.isTrue(cacheHit);
    assert.strictEqual(fetchStub.callCount, 1);
    assert.strictEqual(cachedRes, artifactResponse);
  });
});
