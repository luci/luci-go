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

import { aTimeout, fixture, fixtureCleanup } from '@open-wc/testing-helpers';
import { html, LitElement } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed } from 'mobx';
import { destroy } from 'mobx-state-tree';

import './test_results_tab';
import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import {
  QueryTestVariantsRequest,
  QueryTestVariantsResponse,
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/common/services/resultdb';
import { provideStore, Store, StoreInstance } from '@/common/store';
import { provideInvocationState } from '@/common/store/invocation_state';
import { CacheOption } from '@/generic_libs/tools/cached_fn';
import { provider } from '@/generic_libs/tools/lit_context';
import { timeout } from '@/generic_libs/tools/utils';

import { TestResultsTabElement } from './test_results_tab';

const variant1 = {
  testId: 'a',
  sourcesId: '1',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  statusV2: TestVerdict_Status.FAILED,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant2 = {
  testId: 'a',
  sourcesId: '1',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  statusV2: TestVerdict_Status.FAILED,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant3 = {
  testId: 'a',
  sourcesId: '1',
  variant: { def: { key1: 'val3' } },
  variantHash: 'key1:val3',
  statusV2: TestVerdict_Status.FAILED,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant4 = {
  testId: 'b',
  sourcesId: '1',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  statusV2: TestVerdict_Status.FLAKY,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant5 = {
  testId: 'c',
  sourcesId: '1',
  variant: { def: { key1: 'val2', key2: 'val1' } },
  variantHash: 'key1:val2|key2:val1',
  statusV2: TestVerdict_Status.FLAKY,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

@customElement('milo-test-context-provider')
@provider
class ContextProvider extends LitElement {
  @provideStore()
  store!: StoreInstance;

  @provideInvocationState()
  @computed
  get invocationState() {
    return this.store.invocationPage.invocation;
  }
}

describe('TestResultsTab', () => {
  let store: StoreInstance;

  afterEach(() => {
    fixtureCleanup();
    destroy(store);
  });

  test('should load the first page of test variants when connected', async () => {
    store = Store.create({
      authState: { value: { identity: ANONYMOUS_IDENTITY } },
      invocationPage: { invocationId: 'invocation-id' },
    });
    const queryTestVariantsStub = jest.spyOn(
      store.services.resultDb!,
      'queryTestVariants',
    );
    queryTestVariantsStub.mockResolvedValueOnce({
      testVariants: [variant1, variant2, variant3],
      nextPageToken: 'next',
    });
    queryTestVariantsStub.mockResolvedValueOnce({
      testVariants: [variant4, variant5],
    });
    const getInvocationStub = jest.spyOn(
      store.services.resultDb!,
      'getInvocation',
    );
    getInvocationStub.mockReturnValue(Promise.race([]));

    const provider = await fixture<ContextProvider>(html`
      <milo-test-context-provider .store=${store}>
        <milo-test-results-tab></milo-test-results-tab>
      </milo-test-context-provider>
    `);
    const tab = provider.querySelector<TestResultsTabElement>(
      'milo-test-results-tab',
    )!;

    expect(queryTestVariantsStub.mock.calls.length).toStrictEqual(1);

    tab.disconnectedCallback();
    tab.connectedCallback();

    await aTimeout(0);
    expect(store.invocationPage.invocation.testLoader?.isLoading).toStrictEqual(
      false,
    );
    expect(queryTestVariantsStub.mock.calls.length).toStrictEqual(1);
  });
});

describe('loadNextTestVariants', () => {
  let queryTestVariantsStub: jest.SpiedFunction<
    (
      req: QueryTestVariantsRequest,
      cacheOpt?: CacheOption,
    ) => Promise<QueryTestVariantsResponse>
  >;
  let store: StoreInstance;
  let tab: TestResultsTabElement;

  beforeEach(async () => {
    jest.useFakeTimers();
    store = Store.create({
      authState: { value: { identity: ANONYMOUS_IDENTITY } },
      invocationPage: { invocationId: 'invocation-id' },
    });

    queryTestVariantsStub = jest.spyOn(
      store.services.resultDb!,
      'queryTestVariants',
    );
    queryTestVariantsStub.mockImplementationOnce(async () => {
      await timeout(100);
      return { testVariants: [variant1], nextPageToken: 'next0' };
    });
    queryTestVariantsStub.mockImplementationOnce(async () => {
      await timeout(100);
      return { testVariants: [variant2], nextPageToken: 'next1' };
    });

    const getInvocationStub = jest.spyOn(
      store.services.resultDb!,
      'getInvocation',
    );
    getInvocationStub.mockReturnValue(Promise.race([]));

    const provider = await fixture<ContextProvider>(html`
      <milo-test-context-provider .store=${store}>
        <milo-test-results-tab></milo-test-results-tab>
      </milo-test-context-provider>
    `);
    tab = provider.querySelector<TestResultsTabElement>(
      'milo-test-results-tab',
    )!;
  });

  afterEach(() => {
    fixtureCleanup();
    destroy(store);
    const url = `${window.location.protocol}//${window.location.host}${window.location.pathname}`;
    window.history.replaceState(null, '', url);
    jest.useRealTimers();
  });

  test('should trigger automatic loading when visiting the tab for the first time', async () => {
    expect(store.invocationPage.invocation.testLoader?.isLoading).toStrictEqual(
      true,
    );
    expect(queryTestVariantsStub.mock.calls.length).toStrictEqual(1);

    await jest.runOnlyPendingTimersAsync();
    expect(store.invocationPage.invocation.testLoader?.isLoading).toStrictEqual(
      false,
    );
    expect(queryTestVariantsStub.mock.calls.length).toStrictEqual(1);

    // Disconnect, then reload the tab.
    tab.disconnectedCallback();
    await fixture<ContextProvider>(html`
      <milo-test-context-provider .store=${store}>
        <milo-test-results-tab></milo-test-results-tab>
      </milo-test-context-provider>
    `);

    // Should not trigger loading agin.
    expect(store.invocationPage.invocation.testLoader?.isLoading).toStrictEqual(
      false,
    );
    expect(queryTestVariantsStub.mock.calls.length).toStrictEqual(1);
  });
});
