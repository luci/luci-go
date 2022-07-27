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
import { assert } from 'chai';
import { customElement, html, LitElement } from 'lit-element';
import sinon, { SinonStub } from 'sinon';

import '.';
import { AppState, provideAppState } from '../../context/app_state';
import { InvocationState, provideInvocationState } from '../../context/invocation_state';
import { provideConfigsStore, UserConfigsStore } from '../../context/user_configs';
import { provider } from '../../libs/context';
import {
  QueryTestVariantsRequest,
  QueryTestVariantsResponse,
  ResultDb,
  TestVariantStatus,
} from '../../services/resultdb';
import { TestResultsTabElement } from '.';

const variant1 = {
  testId: 'a',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
};

const variant2 = {
  testId: 'a',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.UNEXPECTED,
};

const variant3 = {
  testId: 'a',
  variant: { def: { key1: 'val3' } },
  variantHash: 'key1:val3',
  status: TestVariantStatus.UNEXPECTED,
};

const variant4 = {
  testId: 'b',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.FLAKY,
};

const variant5 = {
  testId: 'c',
  variant: { def: { key1: 'val2', key2: 'val1' } },
  variantHash: 'key1:val2|key2:val1',
  status: TestVariantStatus.FLAKY,
};

@customElement('milo-test-context-provider')
@provider
class ContextProvider extends LitElement {
  @provideAppState()
  appState!: AppState;

  @provideConfigsStore()
  configsStore!: UserConfigsStore;

  @provideInvocationState()
  invocationState!: InvocationState;
}

describe('Test Results Tab', () => {
  it('should load the first page of test variants when connected', async () => {
    const queryTestVariantsStub = sinon.stub<[QueryTestVariantsRequest], Promise<QueryTestVariantsResponse>>();
    queryTestVariantsStub.onCall(0).resolves({ testVariants: [variant1, variant2, variant3], nextPageToken: 'next' });
    queryTestVariantsStub.onCall(1).resolves({ testVariants: [variant4, variant5] });

    const appState = {
      selectedTabId: '',
      resultDb: {
        queryTestVariants: queryTestVariantsStub as typeof ResultDb.prototype.queryTestVariants,
        getInvocation: (() => Promise.race([])) as typeof ResultDb.prototype.getInvocation,
      },
    } as AppState;
    const configsStore = new UserConfigsStore();
    after(() => configsStore.dispose());

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

    await aTimeout(0);
    assert.isFalse(invocationState.testLoader?.isLoading);
    assert.strictEqual(queryTestVariantsStub.callCount, 1);
  });

  describe('loadNextTestVariants', () => {
    let queryTestVariantsStub: SinonStub<[QueryTestVariantsRequest], Promise<QueryTestVariantsResponse>>;
    let appState: AppState;
    let configsStore: UserConfigsStore;
    let invocationState: InvocationState;
    let tab: TestResultsTabElement;

    beforeEach(async () => {
      queryTestVariantsStub = sinon.stub<[QueryTestVariantsRequest], Promise<QueryTestVariantsResponse>>();
      queryTestVariantsStub.onCall(0).resolves({ testVariants: [variant1], nextPageToken: 'next0' });
      queryTestVariantsStub.onCall(1).resolves({ testVariants: [variant2], nextPageToken: 'next1' });
      queryTestVariantsStub.onCall(2).resolves({ testVariants: [variant3], nextPageToken: 'next2' });
      queryTestVariantsStub.onCall(3).resolves({ testVariants: [variant4], nextPageToken: 'next3' });
      queryTestVariantsStub.onCall(4).resolves({ testVariants: [variant5] });
      appState = {
        selectedTabId: '',
        resultDb: {
          queryTestVariants: queryTestVariantsStub as typeof ResultDb.prototype.queryTestVariants,
          getInvocation: (() => Promise.race([])) as typeof ResultDb.prototype.getInvocation,
        },
      } as AppState;
      configsStore = new UserConfigsStore();

      invocationState = new InvocationState(appState);
      invocationState.invocationId = 'invocation-id';

      const provider = await fixture<ContextProvider>(html`
        <milo-test-context-provider
          .appState=${appState}
          .configsStore=${configsStore}
          .invocationState=${invocationState}
        >
          <milo-test-results-tab></milo-test-results-tab>
        </milo-test-context-provider>
      `);
      tab = provider.querySelector<TestResultsTabElement>('milo-test-results-tab')!;
    });

    afterEach(() => {
      invocationState.dispose();
      configsStore.dispose();
      const url = `${window.location.protocol}//${window.location.host}${window.location.pathname}`;
      window.history.replaceState(null, '', url);
      fixtureCleanup();
    });

    it('should trigger automatic loading when visiting the tab for the first time', async () => {
      assert.isTrue(invocationState.testLoader?.isLoading);
      assert.strictEqual(queryTestVariantsStub.callCount, 1);

      await aTimeout(0);
      assert.isFalse(invocationState.testLoader?.isLoading);
      assert.strictEqual(queryTestVariantsStub.callCount, 1);

      // Disconnect, then reload the tab.
      tab.disconnectedCallback();
      await fixture<ContextProvider>(html`
        <milo-test-context-provider
          .appState=${appState}
          .configsStore=${configsStore}
          .invocationState=${invocationState}
        >
          <milo-test-results-tab></milo-test-results-tab>
        </milo-test-context-provider>
      `);

      // Should not trigger loading agin.
      assert.isFalse(invocationState.testLoader?.isLoading);
      assert.strictEqual(queryTestVariantsStub.callCount, 1);
    });
  });
});
