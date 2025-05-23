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

import { destroy, protect, unprotect } from 'mobx-state-tree';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import {
  QueryTestVariantsRequest,
  QueryTestVariantsResponse,
  TestVariant,
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/common/services/resultdb';
import { Store, StoreInstance } from '@/common/store';
import { logging } from '@/common/tools/logging';
import { CacheOption } from '@/generic_libs/tools/cached_fn';

const variant1: TestVariant = {
  testId: 'invocation-a/test-suite-a/test-1',
  sourcesId: '1',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  statusV2: TestVerdict_Status.FAILED,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant2: TestVariant = {
  testId: 'invocation-a/test-suite-a/test-2',
  sourcesId: '1',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  statusV2: TestVerdict_Status.FAILED,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant3: TestVariant = {
  testId: 'invocation-a/test-suite-b/test-3',
  sourcesId: '1',
  variant: { def: { key1: 'val3' } },
  variantHash: 'key1:val3',
  statusV2: TestVerdict_Status.FLAKY,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant4: TestVariant = {
  testId: 'invocation-a/test-suite-B/test-4',
  sourcesId: '1',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  statusV2: TestVerdict_Status.FAILED,
  statusOverride: TestVerdict_StatusOverride.EXONERATED,
};

const variant5: TestVariant = {
  testId: 'invocation-a/test-suite-B/test-5',
  sourcesId: '1',
  variant: { def: { key1: 'val2', key2: 'val1' } },
  variantHash: 'key1:val2|key2:val1',
  statusV2: TestVerdict_Status.PASSED,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

describe('InvocationState', () => {
  describe('filterVariant', () => {
    let store: StoreInstance;
    let queryTestVariantsStub: jest.SpiedFunction<
      (
        req: QueryTestVariantsRequest,
        cacheOpt?: CacheOption,
      ) => Promise<QueryTestVariantsResponse>
    >;
    let logErrorMock: jest.SpyInstance;

    beforeAll(async () => {
      store = Store.create({
        authState: { value: { identity: ANONYMOUS_IDENTITY } },
        invocationPage: { invocationId: 'invocation-id' },
      });
      unprotect(store);
      queryTestVariantsStub = jest.spyOn(
        store.services.resultDb!,
        'queryTestVariants',
      );
      queryTestVariantsStub.mockResolvedValueOnce({
        testVariants: [variant1, variant2, variant3, variant4, variant5],
      });
      protect(store);
      await store.invocationPage.invocation.testLoader!.loadNextTestVariants();
      logErrorMock = jest.spyOn(logging, 'error').mockImplementation(() => {});
    });
    afterAll(() => {
      destroy(store);
      logErrorMock.mockRestore();
    });

    test('should not filter out anything when search text is empty', () => {
      store.invocationPage.invocation.setSearchText('');
      expect(
        store.invocationPage.invocation.testLoader!.failedTestVariants,
      ).toEqual([variant1, variant2]);
      expect(
        store.invocationPage.invocation.testLoader!
          .passedAndSkippedTestVariants,
      ).toEqual([variant5]);
    });

    test("should filter out variants whose test ID doesn't match the search text", () => {
      store.invocationPage.invocation.setSearchText('test-suite-a');
      expect(
        store.invocationPage.invocation.testLoader!.failedTestVariants,
      ).toEqual([variant1, variant2]);
      expect(
        store.invocationPage.invocation.testLoader!
          .passedAndSkippedTestVariants,
      ).toEqual([]);
    });

    test('search text should be case insensitive', () => {
      store.invocationPage.invocation.setSearchText('test-suite-b');
      expect(
        store.invocationPage.invocation.testLoader!.failedTestVariants,
      ).toEqual([]);
      expect(
        store.invocationPage.invocation.testLoader!
          .passedAndSkippedTestVariants,
      ).toEqual([variant5]);
    });

    test('should preserve the last known valid filter', () => {
      store.invocationPage.invocation.setSearchText('test-suite-b');
      expect(
        store.invocationPage.invocation.testLoader!.failedTestVariants,
      ).toEqual([]);
      expect(
        store.invocationPage.invocation.testLoader!
          .passedAndSkippedTestVariants,
      ).toEqual([variant5]);
      store.invocationPage.invocation.setSearchText('invalid:filter');
      expect(
        store.invocationPage.invocation.testLoader!.failedTestVariants,
      ).toEqual([]);
      expect(
        store.invocationPage.invocation.testLoader!
          .passedAndSkippedTestVariants,
      ).toEqual([variant5]);
    });
  });
});
