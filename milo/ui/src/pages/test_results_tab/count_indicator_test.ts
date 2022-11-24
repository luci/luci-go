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
import { destroy, protect, unprotect } from 'mobx-state-tree';
import sinon from 'sinon';

import './count_indicator';
import { provider } from '../../libs/context';
import { ANONYMOUS_IDENTITY } from '../../services/milo_internal';
import { TestVariantStatus } from '../../services/resultdb';
import { Store } from '../../store';
import { InvocationStateInstance, provideInvocationState } from '../../store/invocation_state';
import { TestResultsTabCountIndicatorElement } from './count_indicator';

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

@customElement('milo-trt-count-indicator-context-provider')
@provider
class ContextProvider extends LitElement {
  @provideInvocationState()
  invocationState!: InvocationStateInstance;
}

describe('Test Count Indicator', () => {
  it('should load the first page of test variants when connected', async () => {
    const store = Store.create({
      authState: { value: { identity: ANONYMOUS_IDENTITY } },
      invocationPage: { invocationId: 'invocation-id' },
    });
    after(() => destroy(store));
    unprotect(store);
    const queryTestVariantsStub = sinon.stub(store.services.resultDb!, 'queryTestVariants');
    protect(store);

    queryTestVariantsStub.onCall(0).resolves({ testVariants: [variant1, variant2, variant3], nextPageToken: 'next' });
    queryTestVariantsStub.onCall(1).resolves({ testVariants: [variant4, variant5] });

    after(fixtureCleanup);
    const provider = await fixture<ContextProvider>(html`
      <milo-trt-count-indicator-context-provider .invocationState=${store.invocationPage.invocation}>
        <milo-trt-count-indicator></milo-trt-count-indicator>
      </milo-trt-count-indicator-context-provider>
    `);
    const indicator = provider.querySelector<TestResultsTabCountIndicatorElement>('milo-trt-count-indicator')!;

    assert.strictEqual(queryTestVariantsStub.callCount, 1);

    indicator.disconnectedCallback();
    indicator.connectedCallback();

    await aTimeout(0);
    assert.isFalse(store.invocationPage.invocation.testLoader?.isLoading);
    assert.strictEqual(queryTestVariantsStub.callCount, 1);
  });
});
