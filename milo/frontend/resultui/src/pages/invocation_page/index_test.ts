// Copyright 2020 The LUCI Authors.
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

import { fixture, fixtureCleanup } from '@open-wc/testing/index-no-side-effects';
import { Commands, RouterLocation } from '@vaadin/router';
import { assert } from 'chai';
import { html } from 'lit-element';
import sinon from 'sinon';

import '.';
import { InvocationPageElement } from '.';
import { AppState } from '../../context/app_state/app_state';
import { DEFAULT_USER_CONFIGS } from '../../context/app_state/user_configs';
import { InvocationState } from '../../context/invocation_state/invocation_state';
import { NOT_FOUND_URL } from '../../routes';


describe('Invocation Page', () => {
  it('should get invocation ID from URL', async () => {
    const appState = new AppState();

    const invocationState = new InvocationState(appState, DEFAULT_USER_CONFIGS);
    after(() => invocationState.dispose());

    after(fixtureCleanup);
    const page = await fixture<InvocationPageElement>(html`
      <milo-invocation-page
        .appState=${appState}
        .invocationState=${invocationState}
      ></milo-invocation-page>
    `);

    const location = {params: {'invocation_id': 'invocation_id'}} as Partial<RouterLocation> as RouterLocation;
    const cmd = {} as Partial<Commands> as Commands;
    await page.onBeforeEnter(location, cmd);
    page.connectedCallback();
    assert.strictEqual(page.invocationState.invocationId, location.params['invocation_id']);
  });

  it('should redirect to the not found page when invocation_id is not provided', async () => {
    const appState = new AppState();

    const invocationState = new InvocationState(appState, DEFAULT_USER_CONFIGS);
    after(() => invocationState.dispose());

    after(fixtureCleanup);
    const page = await fixture<InvocationPageElement>(html`
      <milo-invocation-page
        .appState=${appState}
        .invocationState=${invocationState}
      ></milo-invocation-page>
    `);

    const location = {params: {}} as Partial<RouterLocation> as RouterLocation;
    const redirect = sinon.spy();
    const cmd = {redirect} as Partial<Commands> as Commands;
    await page.onBeforeEnter(location, cmd);
    assert.isTrue(redirect.calledOnceWith(NOT_FOUND_URL));
  });
});
