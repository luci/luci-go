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

import { fixture, fixtureCleanup } from '@open-wc/testing/index-no-side-effects';
import { assert } from 'chai';
import { customElement, html, LitElement, property } from 'lit-element';
import sinon from 'sinon';

import { AppState, provideAppState } from '../context/app_state/app_state';
import { provideConfigsStore, UserConfigsStore } from '../context/app_state/user_configs';
import { InvocationState, provideInvocationState } from '../context/invocation_state/invocation_state';
import { QueryTestVariantsRequest, QueryTestVariantsResponse, TestVariantStatus, UISpecificService } from '../services/resultdb';
import './test_results_tab';
import { TestResultsTabElement } from './test_results_tab';

const variant1 = {
  testId: 'a',
  variant: {'def': {'key1': 'val1'}},
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
};

const variant2 = {
  testId: 'a',
  variant: {def: {'key1': 'val2'}},
  variantHash: 'key1:val2',
  status: TestVariantStatus.UNEXPECTED,
};

const variant3 = {
  testId: 'a',
  variant: {def: {'key1': 'val3'}},
  variantHash: 'key1:val3',
  status: TestVariantStatus.UNEXPECTED,
};

const variant4 = {
  testId: 'b',
  variant: {'def': {'key1': 'val2'}},
  variantHash: 'key1:val2',
  status: TestVariantStatus.FLAKY,
};

const variant5 = {
  testId: 'c',
  variant: {def: {'key1': 'val2', 'key2': 'val1'}},
  variantHash: 'key1:val2|key2:val1',
  status: TestVariantStatus.FLAKY,
};


@customElement('milo-test-context-provider')
@provideConfigsStore
@provideInvocationState
@provideAppState
class ContextProvider extends LitElement {
  @property() appState!: AppState;
  @property() configsStore!: UserConfigsStore;
  @property() invocationState!: InvocationState;
}

describe('Test Results Tab', () => {
  it('should get invocation ID from URL', async () => {
    const queryTestVariantsStub = sinon.stub<[QueryTestVariantsRequest], Promise<QueryTestVariantsResponse>>();
    queryTestVariantsStub.onCall(0).resolves({testVariants: [variant1, variant2, variant3], nextPageToken: 'next'});
    queryTestVariantsStub.onCall(1).resolves({testVariants: [variant4, variant5]});

    const appState = {
      selectedTabId: '',
      uiSpecificService: {
        queryTestVariants: queryTestVariantsStub as typeof UISpecificService.prototype.queryTestVariants,
      },
    } as AppState;
    const configsStore = new UserConfigsStore();

    const invocationState = new InvocationState(appState);
    invocationState.invocationId = 'invocation-id';
    after(() => invocationState.dispose());

    after(fixtureCleanup);
    const provider = await fixture<ContextProvider>(html`
      <milo-test-context-provider
        .appState=${appState}
        .configsStore=${configsStore}
        .invocationState=${invocationState}
      >
        <milo-test-results-tab></milo-test-results-tab>
      </milo-test-context-provider>
    `);
    const tab = provider.querySelector<TestResultsTabElement>('milo-test-results-tab')!;

    assert.strictEqual(queryTestVariantsStub.callCount, 1);

    tab.disconnectedCallback();
    tab.connectedCallback();

    assert.isFalse(invocationState.testLoader?.isLoading);
    assert.strictEqual(queryTestVariantsStub.callCount, 1);
  });
});
