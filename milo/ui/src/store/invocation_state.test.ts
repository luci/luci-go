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

import { afterAll, beforeAll, expect, jest } from '@jest/globals';
import { destroy, protect, unprotect } from 'mobx-state-tree';

import { ANONYMOUS_IDENTITY } from '../libs/auth_state';
import { CacheOption } from '../libs/cached_fn';
import {
  QueryTestVariantsRequest,
  QueryTestVariantsResponse,
  TestVariant,
  TestVariantStatus,
} from '../services/resultdb';
import { Store, StoreInstance } from '.';

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
    let store: StoreInstance;
    let queryTestVariantsStub: jest.SpiedFunction<
      (
        req: QueryTestVariantsRequest,
        cacheOpt?: CacheOption
      ) => Promise<QueryTestVariantsResponse>
    >;
    beforeAll(async () => {
      store = Store.create({
        authState: { value: { identity: ANONYMOUS_IDENTITY } },
        invocationPage: { invocationId: 'invocation-id' },
      });
      unprotect(store);
      queryTestVariantsStub = jest.spyOn(
        store.services.resultDb!,
        'queryTestVariants'
      );
      queryTestVariantsStub.mockResolvedValueOnce({
        testVariants: [variant1, variant2, variant3, variant4, variant5],
      });
      protect(store);
      await store.invocationPage.invocation.testLoader!.loadNextTestVariants();
    });
    afterAll(() => {
      destroy(store);
    });

    it('should not filter out anything when search text is empty', () => {
      store.invocationPage.invocation.setSearchText('');
      expect(
        store.invocationPage.invocation.testLoader!.unexpectedTestVariants
      ).toEqual([variant1, variant2]);
      expect(
        store.invocationPage.invocation.testLoader!.expectedTestVariants
      ).toEqual([variant5]);
    });

    it("should filter out variants whose test ID doesn't match the search text", () => {
      store.invocationPage.invocation.setSearchText('test-suite-a');
      expect(
        store.invocationPage.invocation.testLoader!.unexpectedTestVariants
      ).toEqual([variant1, variant2]);
      expect(
        store.invocationPage.invocation.testLoader!.expectedTestVariants
      ).toEqual([]);
    });

    it('search text should be case insensitive', () => {
      store.invocationPage.invocation.setSearchText('test-suite-b');
      expect(
        store.invocationPage.invocation.testLoader!.unexpectedTestVariants
      ).toEqual([]);
      expect(
        store.invocationPage.invocation.testLoader!.expectedTestVariants
      ).toEqual([variant5]);
    });

    it('should preserve the last known valid filter', () => {
      store.invocationPage.invocation.setSearchText('test-suite-b');
      expect(
        store.invocationPage.invocation.testLoader!.unexpectedTestVariants
      ).toEqual([]);
      expect(
        store.invocationPage.invocation.testLoader!.expectedTestVariants
      ).toEqual([variant5]);
      store.invocationPage.invocation.setSearchText('invalid:filter');
      expect(
        store.invocationPage.invocation.testLoader!.unexpectedTestVariants
      ).toEqual([]);
      expect(
        store.invocationPage.invocation.testLoader!.expectedTestVariants
      ).toEqual([variant5]);
    });
  });
});
