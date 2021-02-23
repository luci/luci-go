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
import { BuildPageElement } from '.';
import { AppState } from '../../context/app_state';
import { UserConfigsStore } from '../../context/user_configs';
import { Build, BuildInput, BuildsService } from '../../services/buildbucket';
import { getInvIdFromBuildId, getInvIdFromBuildNum, ResultDb } from '../../services/resultdb';

const builder = {
  project: 'project',
  bucket: 'bucket',
  builder: 'builder',
};

describe('Invocation Page', () => {
  it('should compute invocation ID from buildNum in URL', async () => {
    after(fixtureCleanup);
    const page = await fixture<BuildPageElement>(html`
      <milo-build-page
        .prerender=${true}
        .appState=${new AppState()}
        .configsStore=${new UserConfigsStore()}
      ></milo-build-page>
    `);

    const location = {
      params: {
        ...builder,
        'build_num_or_id': '1234',
      },
    } as Partial<RouterLocation> as RouterLocation;
    const cmd = {} as Partial<Commands> as Commands;
    await page.onBeforeEnter(location, cmd);
    page.connectedCallback();
    await aTimeout(10);
    assert.strictEqual(page.buildState.invocationId, await getInvIdFromBuildNum(builder, 1234));
    page.disconnectedCallback();
  });

  it('should compute invocation ID from build ID in URL', async () => {
    after(fixtureCleanup);
    const page = await fixture<BuildPageElement>(html`
      <milo-build-page
        .prerender=${true}
        .appState=${new AppState()}
        .configsStore=${new UserConfigsStore()}
      ></milo-build-page>
    `);

    const location = {
      params: {
        ...builder,
        'build_num_or_id': 'b1234',
      },
    } as Partial<RouterLocation> as RouterLocation;
    const cmd = {} as Partial<Commands> as Commands;
    await page.onBeforeEnter(location, cmd);
    page.connectedCallback();
    await aTimeout(10);
    assert.strictEqual(page.buildState.invocationId, getInvIdFromBuildId('1234'));
    page.disconnectedCallback();
  });

  it('should fallback to invocation ID from buildbucket when invocation is not found', async () => {
    const resultDbStub = sinon.stub(new ResultDb('', ''));
    const buildsServiceStub = sinon.stub(new BuildsService('', ''));
    resultDbStub?.getInvocation.onCall(0).rejects();
    resultDbStub?.getInvocation.onCall(1).resolves();
    buildsServiceStub?.getBuild.onCall(0).resolves({
      infra: {
        resultdb: {
          hostname: 'hostname',
          invocation: 'invocations/invocation-id',
        },
      },
      input: {
        gitilesCommit: {},
      } as BuildInput,
      output: {
        properties: {},
      },
    } as Build);

    after(fixtureCleanup);
    const page = await fixture<BuildPageElement>(html`
      <milo-build-page
        .prerender=${true}
        .appState=${{
          ...new AppState(),
          resultDb: resultDbStub,
          buildsService: buildsServiceStub,
        }}
        .configsStore=${new UserConfigsStore()}
      ></milo-build-page>
    `);

    const location = {
      params: {
        ...builder,
        'build_num_or_id': 'b1234',
      },
    } as Partial<RouterLocation> as RouterLocation;
    const cmd = {} as Partial<Commands> as Commands;

    await page.onBeforeEnter(location, cmd);
    page.connectedCallback();
    await aTimeout(100);
    assert.strictEqual(page.buildState.invocationId, 'invocation-id');
    page.disconnectedCallback();
  });
});
