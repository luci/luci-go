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

import { beforeEach, expect, jest } from '@jest/globals';

import { CacheOption } from '../libs/cached_fn';
import {
  createTVPropGetter,
  QueryTestVariantsRequest,
  QueryTestVariantsResponse,
  ResultDb,
  TestVariantStatus,
} from '../services/resultdb';
import { LoadingStage, TestLoader } from './test_loader';

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
  status: TestVariantStatus.UNEXPECTEDLY_SKIPPED,
};

const variant4 = {
  testId: 'b',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.FLAKY,
};

const variant5 = {
  testId: 'matched-id',
  variant: { def: { key1: 'val2', key2: 'val1' } },
  variantHash: 'key1:val2|key2:val1',
  status: TestVariantStatus.FLAKY,
};

const variant6 = {
  testId: 'c',
  variant: { def: { key1: 'val2', key2: 'val2' } },
  variantHash: 'key1:val2|key2:val2',
  status: TestVariantStatus.EXONERATED,
};

const variant7 = {
  testId: 'd',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.EXONERATED,
};

const variant8 = {
  testId: 'd',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.EXPECTED,
};

const variant9 = {
  testId: 'e',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.EXPECTED,
};

const variant10 = {
  testId: 'f',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.EXPECTED,
};

const variant11 = {
  testId: 'g',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.EXPECTED,
};

const variant12 = {
  testId: 'matched-id',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.EXPECTED,
};

describe('TestLoader', () => {
  describe('when first page contains variants', () => {
    let testLoader: TestLoader;
    let stub: jest.Mock<(req: QueryTestVariantsRequest, cacheOpt?: CacheOption) => Promise<QueryTestVariantsResponse>>;
    const req = { invocations: ['invocation'], pageSize: 4 };

    beforeEach(() => {
      stub = jest.fn<(req: QueryTestVariantsRequest, cacheOpt?: CacheOption) => Promise<QueryTestVariantsResponse>>();
      stub.mockResolvedValueOnce({ testVariants: [variant1, variant2, variant3, variant4], nextPageToken: 'page2' });
      stub.mockResolvedValueOnce({ testVariants: [variant5, variant6, variant7], nextPageToken: 'page3' });
      stub.mockResolvedValueOnce({ testVariants: [variant8, variant9, variant10, variant11], nextPageToken: 'page4' });
      stub.mockResolvedValueOnce({ testVariants: [variant12], nextPageToken: undefined });
      testLoader = new TestLoader(req, {
        queryTestVariants: stub,
      } as Partial<ResultDb> as ResultDb);
    });

    it('should preserve loading progress', async () => {
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingUnexpected);
      expect(stub.mock.calls.length).toStrictEqual(0);

      await testLoader.loadNextTestVariants();
      expect(testLoader.unexpectedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonExpectedTestVariants).toEqual([variant1, variant2, variant3, variant4]);
      expect(testLoader.expectedTestVariants).toEqual([]);
      expect(testLoader.unfilteredUnexpectedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredUnexpectedlySkippedVariantsCount).toStrictEqual(1);
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(1);
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingFlaky);
      expect(stub.mock.calls.length).toStrictEqual(1);
      expect(stub.mock.lastCall?.[0]).toEqual({ ...req, pageSize: 10000, pageToken: '' });

      await testLoader.loadNextTestVariants();
      expect(testLoader.unexpectedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonExpectedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.expectedTestVariants).toEqual([]);
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingExpected);
      expect(stub.mock.calls.length).toStrictEqual(2);
      expect(stub.mock.lastCall?.[0]).toEqual({ ...req, pageSize: 10000, pageToken: 'page2' });

      await testLoader.loadNextTestVariants();
      expect(testLoader.unexpectedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonExpectedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.expectedTestVariants).toEqual([variant8, variant9, variant10, variant11]);
      expect(testLoader.unfilteredUnexpectedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredUnexpectedlySkippedVariantsCount).toStrictEqual(1);
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingExpected);
      expect(stub.mock.calls.length).toStrictEqual(3);
      expect(stub.mock.lastCall?.[0]).toEqual({ ...req, pageSize: 10000, pageToken: 'page3' });

      await testLoader.loadNextTestVariants();
      expect(testLoader.unexpectedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonExpectedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.expectedTestVariants).toEqual([variant8, variant9, variant10, variant11, variant12]);
      expect(testLoader.unfilteredUnexpectedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredUnexpectedlySkippedVariantsCount).toStrictEqual(1);
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(LoadingStage.Done);
      expect(stub.mock.calls.length).toStrictEqual(4);
      expect(stub.mock.lastCall?.[0]).toEqual({ ...req, pageSize: 10000, pageToken: 'page4' });

      // Should not load when the iterator is exhausted.
      await testLoader.loadNextTestVariants();
      expect(testLoader.unexpectedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonExpectedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.expectedTestVariants).toEqual([variant8, variant9, variant10, variant11, variant12]);
      expect(testLoader.unfilteredUnexpectedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredUnexpectedlySkippedVariantsCount).toStrictEqual(1);
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(LoadingStage.Done);
      expect(stub.mock.calls.length).toStrictEqual(4);
    });

    it('should handle concurrent loadNextPage calls correctly', async () => {
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingUnexpected);

      const loadReq1 = testLoader.loadNextTestVariants();
      const loadReq2 = testLoader.loadNextTestVariants();
      const loadReq3 = testLoader.loadNextTestVariants();
      const loadReq4 = testLoader.loadNextTestVariants();
      const loadReq5 = testLoader.loadNextTestVariants();
      expect(testLoader.isLoading).toBeTruthy();
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingUnexpected);

      await loadReq1;
      expect(testLoader.unexpectedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonExpectedTestVariants).toEqual([variant1, variant2, variant3, variant4]);
      expect(testLoader.expectedTestVariants).toEqual([]);
      // loadReq2 has not finished loading yet.
      expect(testLoader.isLoading).toBeTruthy();
      expect(testLoader.unfilteredUnexpectedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredUnexpectedlySkippedVariantsCount).toStrictEqual(1);
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(1);
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingFlaky);

      await loadReq2;
      expect(testLoader.unexpectedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonExpectedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.expectedTestVariants).toEqual([]);
      // loadReq3 has not finished loading yet.
      expect(testLoader.isLoading).toBeTruthy();
      expect(testLoader.unfilteredUnexpectedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredUnexpectedlySkippedVariantsCount).toStrictEqual(1);
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingExpected);

      await loadReq3;
      expect(testLoader.unexpectedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonExpectedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.expectedTestVariants).toEqual([variant8, variant9, variant10, variant11]);
      // loadReq4 has not finished loading yet.
      expect(testLoader.isLoading).toBeTruthy();
      expect(testLoader.unfilteredUnexpectedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredUnexpectedlySkippedVariantsCount).toStrictEqual(1);
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingExpected);

      await loadReq4;
      expect(testLoader.unexpectedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonExpectedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.expectedTestVariants).toEqual([variant8, variant9, variant10, variant11, variant12]);
      // The list is exhausted, loadReq5 should not change the loading state.
      expect(testLoader.isLoading).toBeFalsy();
      expect(testLoader.unfilteredUnexpectedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredUnexpectedlySkippedVariantsCount).toStrictEqual(1);
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(LoadingStage.Done);

      await loadReq5;
      expect(testLoader.unexpectedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonExpectedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.expectedTestVariants).toEqual([variant8, variant9, variant10, variant11, variant12]);
      expect(testLoader.isLoading).toBeFalsy();
      expect(testLoader.unfilteredUnexpectedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredUnexpectedlySkippedVariantsCount).toStrictEqual(1);
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(LoadingStage.Done);

      expect(stub.mock.calls.length).toStrictEqual(4);
      expect(stub.mock.calls[0][0]).toEqual({ ...req, pageSize: 10000, pageToken: '' });
      expect(stub.mock.calls[1][0]).toEqual({ ...req, pageSize: 10000, pageToken: 'page2' });
      expect(stub.mock.calls[2][0]).toEqual({ ...req, pageSize: 10000, pageToken: 'page3' });
      expect(stub.mock.calls[3][0]).toEqual({ ...req, pageSize: 10000, pageToken: 'page4' });
    });

    it('loadFirstPageOfTestVariants should work correctly', async () => {
      const firstLoadPromise = testLoader.loadNextTestVariants();
      expect(firstLoadPromise).toStrictEqual(testLoader.loadFirstPageOfTestVariants());
      const secondLoadPromise = testLoader.loadNextTestVariants();
      expect(secondLoadPromise).not.toBe(testLoader.loadFirstPageOfTestVariants());
    });

    it('should load at least one test variant that matches the filter', async () => {
      testLoader.filter = (v) => v.testId === 'matched-id';
      await testLoader.loadNextTestVariants();

      expect(testLoader.unexpectedTestVariants).toEqual([]);
      expect(testLoader.nonExpectedTestVariants).toEqual([variant5]);
      expect(testLoader.expectedTestVariants).toEqual([]);

      await testLoader.loadNextTestVariants();

      expect(testLoader.unexpectedTestVariants).toEqual([]);
      expect(testLoader.nonExpectedTestVariants).toEqual([variant5]);
      expect(testLoader.expectedTestVariants).toEqual([variant12]);
    });

    it('should load at least one test variant that matches the filter', async () => {
      testLoader.filter = (v) => v.testId === 'matched-id';
      await testLoader.loadNextTestVariants();

      expect(testLoader.unexpectedTestVariants).toEqual([]);
      expect(testLoader.nonExpectedTestVariants).toEqual([variant5]);
      expect(testLoader.expectedTestVariants).toEqual([]);

      await testLoader.loadNextTestVariants();

      expect(testLoader.unexpectedTestVariants).toEqual([]);
      expect(testLoader.nonExpectedTestVariants).toEqual([variant5]);
      expect(testLoader.expectedTestVariants).toEqual([variant12]);
    });

    it('should stop loading at the final page when no test variants matches the filter', async () => {
      testLoader.filter = () => false;

      // Detect infinite loop and abort.
      const oldLoadNextTestVariants = testLoader.loadNextTestVariants.bind(testLoader);
      let callCount = 0;
      testLoader.loadNextTestVariants = (...params) => {
        callCount++;
        if (callCount > 10) {
          throw new Error('too many load next page calls');
        }
        return oldLoadNextTestVariants(...params);
      };

      await testLoader.loadNextTestVariants();

      expect(testLoader.testVariantCount).toStrictEqual(0);
      expect(testLoader.unfilteredTestVariantCount).toStrictEqual(12);
      expect(testLoader.isLoading).toBeFalsy();
      expect(testLoader.stage).toStrictEqual(LoadingStage.Done);
    });
  });

  describe('when first page contains no variants', () => {
    let testLoader: TestLoader;
    let stub: jest.Mock<(req: QueryTestVariantsRequest, cacheOpt?: CacheOption) => Promise<QueryTestVariantsResponse>>;
    const req = { invocations: ['invocation'], pageSize: 4 };

    beforeEach(() => {
      stub = jest.fn<(req: QueryTestVariantsRequest, cacheOpt?: CacheOption) => Promise<QueryTestVariantsResponse>>();
      stub.mockResolvedValueOnce({ nextPageToken: 'page2' });
      stub.mockResolvedValueOnce({ testVariants: [variant8], nextPageToken: 'page3' });
      stub.mockResolvedValueOnce({ testVariants: [variant9], nextPageToken: undefined });
      testLoader = new TestLoader(req, {
        queryTestVariants: stub,
      } as Partial<ResultDb> as ResultDb);
    });

    it('should correctly handle a response with 0 variants', async () => {
      expect(stub.mock.calls.length).toStrictEqual(0);

      await testLoader.loadNextTestVariants();
      expect(testLoader.unexpectedTestVariants).toEqual([]);
      expect(testLoader.nonExpectedTestVariants).toEqual([]);
      expect(testLoader.expectedTestVariants).toEqual([variant8]);
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingExpected);
    });
  });

  describe('when grouping test variants', () => {
    let testLoader: TestLoader;
    let stub: jest.Mock<(req: QueryTestVariantsRequest, cacheOpt?: CacheOption) => Promise<QueryTestVariantsResponse>>;
    const req = { invocations: ['invocation'], pageSize: 4 };

    beforeEach(() => {
      stub = jest.fn<(req: QueryTestVariantsRequest, cacheOpt?: CacheOption) => Promise<QueryTestVariantsResponse>>();
      stub.mockResolvedValueOnce({
        testVariants: [variant1, variant2, variant3, variant4, variant5, variant6, variant7],
        nextPageToken: 'page1',
      });
      stub.mockResolvedValueOnce({ testVariants: [variant8, variant9, variant10, variant11, variant12] });
      testLoader = new TestLoader(req, {
        queryTestVariants: stub,
      } as Partial<ResultDb> as ResultDb);
    });

    it('should not return empty groups', async () => {
      // [] means there are no groups. [[]] means there is one empty group.
      expect(testLoader.groupedNonExpectedVariants).toEqual([]);
    });

    it('should group test variants correctly', async () => {
      testLoader.groupers = [['status', createTVPropGetter('status')]];
      await testLoader.loadNextTestVariants();
      await testLoader.loadNextTestVariants();

      expect(testLoader.groupedNonExpectedVariants).toEqual([
        [variant1, variant2],
        [variant3],
        [variant4, variant5],
        [variant6, variant7],
      ]);
      expect(testLoader.expectedTestVariants).toEqual([variant8, variant9, variant10, variant11, variant12]);
    });

    it('should support multiple grouping keys', async () => {
      testLoader.groupers = [
        ['status', createTVPropGetter('status')],
        ['v.key1', createTVPropGetter('v.key1')],
      ];
      await testLoader.loadNextTestVariants();
      await testLoader.loadNextTestVariants();

      expect(testLoader.groupedNonExpectedVariants).toEqual([
        [variant1],
        [variant2],
        [variant3],
        [variant4, variant5],
        [variant6],
        [variant7],
      ]);
      expect(testLoader.expectedTestVariants).toEqual([variant8, variant9, variant10, variant11, variant12]);
    });
  });
});
