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

import { aTimeout, fixture, fixtureCleanup } from '@open-wc/testing/index-no-side-effects';
import { Commands, RouterLocation } from '@vaadin/router';
import { assert } from 'chai';
import { html } from 'lit-element';
import sinon from 'sinon';

import '.';
import { AppState } from '../../context/app_state';
import { UserConfigsStore } from '../../context/user_configs';
import { PrpcClientExt } from '../../libs/prpc_client_ext';
import { Build, BuildsService, GetBuildRequest } from '../../services/buildbucket';
import { getInvIdFromBuildId, getInvIdFromBuildNum, ResultDb } from '../../services/resultdb';
import { BuildPageElement } from '.';

const builder = {
  project: 'project',
  bucket: 'bucket',
  builder: 'builder',
};

describe('Build Page', () => {
  let configsStore: UserConfigsStore;
  let appState: AppState;

  beforeEach(async () => {
    configsStore = new UserConfigsStore();
    appState = new AppState();
  });

  afterEach(async () => {
    appState.dispose();
    fixtureCleanup();
  });

  it('should compute invocation ID from buildNum in URL', async () => {
    const pageContainer = await fixture(html`
      <div>
        <milo-build-page .prerender=${true} .appState=${new AppState()} .configsStore=${configsStore}></milo-build-page>
      </div>
    `);
    const page = pageContainer.querySelector<BuildPageElement>('milo-build-page')!;
    pageContainer.removeChild(page);

    const location = {
      params: {
        ...builder,
        build_num_or_id: '1234',
      },
    } as Partial<RouterLocation> as RouterLocation;
    const cmd = {} as Partial<Commands> as Commands;
    page.prerender = false;
    await page.onBeforeEnter(location, cmd);
    pageContainer.appendChild(page);
    await aTimeout(0);
    assert.strictEqual(page.buildState.invocationId, await getInvIdFromBuildNum(builder, 1234));
  });

  it('should compute invocation ID from build ID in URL', async () => {
    const pageContainer = await fixture(html`
      <div>
        <milo-build-page .prerender=${true} .appState=${new AppState()} .configsStore=${configsStore}></milo-build-page>
      </div>
    `);
    const page = pageContainer.querySelector<BuildPageElement>('milo-build-page')!;
    pageContainer.removeChild(page);

    const location = {
      params: {
        ...builder,
        build_num_or_id: 'b1234',
      },
    } as Partial<RouterLocation> as RouterLocation;
    const cmd = {} as Partial<Commands> as Commands;
    page.prerender = false;
    await page.onBeforeEnter(location, cmd);
    pageContainer.appendChild(page);
    await aTimeout(0);
    assert.strictEqual(page.buildState.invocationId, getInvIdFromBuildId('1234'));
  });

  it('should fallback to invocation ID from buildbucket when invocation is not found', async () => {
    const resultDbStub = sinon.stub(new ResultDb(new PrpcClientExt({}, () => '')));
    const buildsServiceStub = sinon.stub(new BuildsService(new PrpcClientExt({}, () => '')));
    resultDbStub?.getInvocation.onCall(0).rejects();
    resultDbStub?.getInvocation.onCall(1).resolves();
    buildsServiceStub?.getBuild.onCall(0).resolves({
      infra: {
        resultdb: {
          hostname: 'hostname',
          invocation: 'invocations/invocation-id',
        },
      },
      input: { properties: {} },
      output: {
        properties: {},
      },
    } as Build);

    const pageContainer = await fixture<BuildPageElement>(html`
      <div>
        <milo-build-page
          .prerender=${true}
          .appState=${{
            ...appState,
            resultDb: resultDbStub,
            buildsService: buildsServiceStub,
          }}
          .configsStore=${configsStore}
        ></milo-build-page>
      </div>
    `);
    const page = pageContainer.querySelector<BuildPageElement>('milo-build-page')!;
    pageContainer.removeChild(page);

    const location = {
      params: {
        ...builder,
        build_num_or_id: 'b1234',
      },
    } as Partial<RouterLocation> as RouterLocation;
    const cmd = {} as Partial<Commands> as Commands;

    page.prerender = false;
    await page.onBeforeEnter(location, cmd);
    pageContainer.appendChild(page);
    await aTimeout(0);
    assert.strictEqual(page.buildState.invocationId, 'invocation-id');
  });

  it('should redirect to a long link when visited via a short link', async () => {
    const original = window.location.href;
    after(() => window.history.replaceState(original, '', original));

    const getBuildMock = sinon.stub<[GetBuildRequest], Promise<Build>>();
    getBuildMock.onCall(0).resolves({
      builder,
      number: 123,
      id: '4567',
      input: { properties: {} },
      output: { properties: {} },
    } as Build);

    const pageContainer = await fixture<BuildPageElement>(html`
      <div>
        <milo-build-page
          .prerender=${true}
          .appState=${{
            ...appState,
            setBuildId: () => {},
            getBuildId: () => '4567',
            buildsService: {
              ...new BuildsService(new PrpcClientExt({}, () => '')),
              getBuild: getBuildMock,
            },
          }}
          .configsStore=${configsStore}
        ></milo-build-page>
      </div>
    `);
    const page = pageContainer.querySelector<BuildPageElement>('milo-build-page')!;
    pageContainer.removeChild(page);

    const location = {
      params: {
        build_id: '4567',
        path: ['test-results'],
      },
      search: '?q=a',
      hash: '#an-element',
    } as Partial<RouterLocation> as RouterLocation;
    const cmd = {} as Partial<Commands> as Commands;

    page.prerender = false;
    await page.onBeforeEnter(location, cmd);
    pageContainer.appendChild(page);
    await aTimeout(20);
    assert.isTrue(
      window.location.href.endsWith('/ui/p/project/builders/bucket/builder/123/test-results?q=a#an-element')
    );
  });
});
