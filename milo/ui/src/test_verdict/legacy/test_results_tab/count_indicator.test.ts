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
import { destroy, protect, unprotect } from 'mobx-state-tree';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { TestVariantStatus } from '@/common/services/resultdb';
import { Store, StoreInstance } from '@/common/store';
import {
  InvocationStateInstance,
  provideInvocationState,
} from '@/common/store/invocation_state';
import { provider } from '@/generic_libs/tools/lit_context';

import './count_indicator';
import { TestResultsTabCountIndicatorElement } from './count_indicator';

const variant1 = {
  testId: 'a',
  sourcesId: '1',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
};

const variant2 = {
  testId: 'a',
  sourcesId: '1',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.UNEXPECTED,
};

const variant3 = {
  testId: 'a',
  sourcesId: '1',
  variant: { def: { key1: 'val3' } },
  variantHash: 'key1:val3',
  status: TestVariantStatus.UNEXPECTED,
};

const variant4 = {
  testId: 'b',
  sourcesId: '1',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.FLAKY,
};

const variant5 = {
  testId: 'c',
  sourcesId: '1',
  variant: { def: { key1: 'val2', key2: 'val1' } },
  variantHash: 'key1:val2|key2:val1',
  status: TestVariantStatus.FLAKY,
};

@customElement('milo-trt-count-indicator-context-provider')
@provider
class ContextProvider extends LitElement {
  @provideInvocationState()
  invocationState!: InvocationStateInstance;
}

describe('CountIndicator', () => {
  let store: StoreInstance;

  beforeEach(() => {
    store = Store.create({
      authState: { value: { identity: ANONYMOUS_IDENTITY } },
      invocationPage: { invocationId: 'invocation-id' },
    });
  });
  afterEach(() => {
    fixtureCleanup();
    destroy(store);
  });

  test('should load the first page of test variants when connected', async () => {
    unprotect(store);
    const queryTestVariantsStub = jest.spyOn(
      store.services.resultDb!,
      'queryTestVariants',
    );
    protect(store);

    queryTestVariantsStub.mockResolvedValueOnce({
      testVariants: [variant1, variant2, variant3],
      nextPageToken: 'next',
    });
    queryTestVariantsStub.mockResolvedValueOnce({
      testVariants: [variant4, variant5],
    });

    const provider = await fixture<ContextProvider>(html`
      <milo-trt-count-indicator-context-provider
        .invocationState=${store.invocationPage.invocation}
      >
        <milo-trt-count-indicator></milo-trt-count-indicator>
      </milo-trt-count-indicator-context-provider>
    `);
    const indicator =
      provider.querySelector<TestResultsTabCountIndicatorElement>(
        'milo-trt-count-indicator',
      )!;

    expect(queryTestVariantsStub.mock.calls.length).toStrictEqual(1);

    indicator.disconnectedCallback();
    indicator.connectedCallback();

    await aTimeout(0);
    expect(store.invocationPage.invocation.testLoader?.isLoading).toStrictEqual(
      false,
    );
    expect(queryTestVariantsStub.mock.calls.length).toStrictEqual(1);
  });
});
