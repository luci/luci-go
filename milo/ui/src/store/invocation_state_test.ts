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

import { assert } from 'chai';
import { destroy, protect, unprotect } from 'mobx-state-tree';
import sinon from 'sinon';

import { ANONYMOUS_IDENTITY } from '../services/milo_internal';
import { TestVariant, TestVariantStatus } from '../services/resultdb';
import { Store } from '.';

const variant1: TestVariant = {
  testId: 'invocation-a/test-suite-a/test-1',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
};

const variant2: TestVariant = {
  testId: 'invocation-a/test-suite-a/test-2',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.UNEXPECTED,
};

const variant3: TestVariant = {
  testId: 'invocation-a/test-suite-b/test-3',
  variant: { def: { key1: 'val3' } },
  variantHash: 'key1:val3',
  status: TestVariantStatus.FLAKY,
};

const variant4: TestVariant = {
  testId: 'invocation-a/test-suite-B/test-4',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.EXONERATED,
};

const variant5: TestVariant = {
  testId: 'invocation-a/test-suite-B/test-5',
  variant: { def: { key1: 'val2', key2: 'val1' } },
  variantHash: 'key1:val2|key2:val1',
  status: TestVariantStatus.EXPECTED,
};

describe('InvocationState', () => {
  describe('filterVariant', () => {
    const store = Store.create({
      authState: { value: { identity: ANONYMOUS_IDENTITY } },
      invocationPage: { invocationId: 'invocation-id' },
    });
    after(() => destroy(store));
    unprotect(store);
    const queryTestVariantsStub = sinon.stub(store.services.resultDb!, 'queryTestVariants');
    protect(store);

    queryTestVariantsStub.onCall(0).resolves({ testVariants: [variant1, variant2, variant3, variant4, variant5] });

    before(async () => await store.invocationPage.invocation.testLoader!.loadNextTestVariants());

    it('should not filter out anything when search text is empty', () => {
      store.invocationPage.invocation.setSearchText('');
      assert.deepEqual(store.invocationPage.invocation.testLoader!.unexpectedTestVariants, [variant1, variant2]);
      assert.deepEqual(store.invocationPage.invocation.testLoader!.expectedTestVariants, [variant5]);
    });

    it("should filter out variants whose test ID doesn't match the search text", () => {
      store.invocationPage.invocation.setSearchText('test-suite-a');
      assert.deepEqual(store.invocationPage.invocation.testLoader!.unexpectedTestVariants, [variant1, variant2]);
      assert.deepEqual(store.invocationPage.invocation.testLoader!.expectedTestVariants, []);
    });

    it('search text should be case insensitive', () => {
      store.invocationPage.invocation.setSearchText('test-suite-b');
      assert.deepEqual(store.invocationPage.invocation.testLoader!.unexpectedTestVariants, []);
      assert.deepEqual(store.invocationPage.invocation.testLoader!.expectedTestVariants, [variant5]);
    });

    it('should preserve the last known valid filter', () => {
      store.invocationPage.invocation.setSearchText('test-suite-b');
      assert.deepEqual(store.invocationPage.invocation.testLoader!.unexpectedTestVariants, []);
      assert.deepEqual(store.invocationPage.invocation.testLoader!.expectedTestVariants, [variant5]);
      store.invocationPage.invocation.setSearchText('invalid:filter');
      assert.deepEqual(store.invocationPage.invocation.testLoader!.unexpectedTestVariants, []);
      assert.deepEqual(store.invocationPage.invocation.testLoader!.expectedTestVariants, [variant5]);
    });
  });
});
