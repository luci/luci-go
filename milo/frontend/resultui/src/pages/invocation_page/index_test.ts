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
import { AppState } from '../../context/app_state';
import { NOT_FOUND_URL } from '../../routes';
import { InvocationPageElement } from '.';

describe('Invocation Page', () => {
  let appState: AppState;
  let pageContainer: HTMLDivElement;
  let page: InvocationPageElement;

  beforeEach(async () => {
    appState = new AppState();
    pageContainer = await fixture(html`
      <div>
        <milo-invocation-page .appState=${appState}></milo-invocation-page>
      </div>
    `);
    page = pageContainer.querySelector<InvocationPageElement>('milo-invocation-page')!;
    pageContainer.removeChild(page);
  });

  afterEach(async () => {
    appState.dispose();
    fixtureCleanup();
  });

  it('should get invocation ID from URL', async () => {
    const location = ({ params: { invocation_id: 'invocation_id' } } as Partial<RouterLocation>) as RouterLocation;
    const cmd = ({} as Partial<Commands>) as Commands;
    await page.onBeforeEnter(location, cmd);
    pageContainer.appendChild(page);
    assert.strictEqual(page.invocationState.invocationId, location.params['invocation_id'] as string);
  });

  it('should redirect to the not found page when invocation_id is not provided', async () => {
    const location = ({ params: {} } as Partial<RouterLocation>) as RouterLocation;
    const redirect = sinon.spy();
    const cmd = ({ redirect } as Partial<Commands>) as Commands;
    await page.onBeforeEnter(location, cmd);
    pageContainer.appendChild(page);
    assert.isTrue(redirect.calledOnceWith(NOT_FOUND_URL));
  });
});
