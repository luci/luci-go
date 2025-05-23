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

import {
  createTVPropGetter,
  QueryTestVariantsRequest,
  QueryTestVariantsResponse,
  ResultDb,
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/common/services/resultdb';
import { CacheOption } from '@/generic_libs/tools/cached_fn';

import { LoadingStage, TestLoader } from './test_loader';

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
  statusV2: TestVerdict_Status.EXECUTION_ERRORED,
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
  testId: 'matched-id',
  sourcesId: '1',
  variant: { def: { key1: 'val2', key2: 'val1' } },
  variantHash: 'key1:val2|key2:val1',
  statusV2: TestVerdict_Status.FLAKY,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant6 = {
  testId: 'c',
  sourcesId: '1',
  variant: { def: { key1: 'val2', key2: 'val2' } },
  variantHash: 'key1:val2|key2:val2',
  statusV2: TestVerdict_Status.FAILED,
  statusOverride: TestVerdict_StatusOverride.EXONERATED,
};

const variant7 = {
  testId: 'd',
  sourcesId: '1',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  statusV2: TestVerdict_Status.PRECLUDED,
  statusOverride: TestVerdict_StatusOverride.EXONERATED,
};

const variant8 = {
  testId: 'd',
  sourcesId: '1',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  statusV2: TestVerdict_Status.PASSED,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant9 = {
  testId: 'e',
  sourcesId: '1',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  statusV2: TestVerdict_Status.PASSED,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant10 = {
  testId: 'f',
  sourcesId: '1',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  statusV2: TestVerdict_Status.SKIPPED,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant11 = {
  testId: 'g',
  sourcesId: '1',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  statusV2: TestVerdict_Status.PASSED,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

const variant12 = {
  testId: 'matched-id',
  sourcesId: '1',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  statusV2: TestVerdict_Status.PASSED,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
};

describe('TestLoader', () => {
  describe('when first page contains variants', () => {
    let testLoader: TestLoader;
    let stub: jest.MockedFunction<
      (
        req: QueryTestVariantsRequest,
        cacheOpt?: CacheOption,
      ) => Promise<QueryTestVariantsResponse>
    >;
    const req = { invocations: ['invocation'], pageSize: 4 };

    beforeEach(() => {
      stub = jest.fn();
      stub.mockResolvedValueOnce({
        testVariants: [variant1, variant2, variant3, variant4],
        nextPageToken: 'page2',
      });
      stub.mockResolvedValueOnce({
        testVariants: [variant5, variant6, variant7],
        nextPageToken: 'page3',
      });
      stub.mockResolvedValueOnce({
        testVariants: [variant8, variant9, variant10, variant11],
        nextPageToken: 'page4',
      });
      stub.mockResolvedValueOnce({
        testVariants: [variant12],
        nextPageToken: undefined,
      });
      testLoader = new TestLoader(req, {
        queryTestVariants: stub,
      } as Partial<ResultDb> as ResultDb);
    });

    test('should preserve loading progress', async () => {
      expect(testLoader.stage).toStrictEqual(
        LoadingStage.LoadingFailedVerdicts,
      );
      expect(stub.mock.calls.length).toStrictEqual(0);

      await testLoader.loadNextTestVariants();
      expect(testLoader.failedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([]);
      expect(testLoader.unfilteredFailedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredExecutionErroredVariantsCount).toStrictEqual(
        1,
      );
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(1);
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingFlakyVerdicts);
      expect(stub.mock.calls.length).toStrictEqual(1);
      expect(stub.mock.lastCall?.[0]).toEqual({
        ...req,
        pageSize: 10000,
        pageToken: '',
        orderBy: 'status_v2_effective',
      });

      await testLoader.loadNextTestVariants();
      expect(testLoader.failedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([]);
      expect(testLoader.stage).toStrictEqual(
        LoadingStage.LoadingPassedAndSkippedVerdicts,
      );
      expect(stub.mock.calls.length).toStrictEqual(2);
      expect(stub.mock.lastCall?.[0]).toEqual({
        ...req,
        pageSize: 10000,
        pageToken: 'page2',
        orderBy: 'status_v2_effective',
      });

      await testLoader.loadNextTestVariants();
      expect(testLoader.failedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([
        variant8,
        variant9,
        variant10,
        variant11,
      ]);
      expect(testLoader.unfilteredFailedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredExecutionErroredVariantsCount).toStrictEqual(
        1,
      );
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(
        LoadingStage.LoadingPassedAndSkippedVerdicts,
      );
      expect(stub.mock.calls.length).toStrictEqual(3);
      expect(stub.mock.lastCall?.[0]).toEqual({
        ...req,
        pageSize: 10000,
        pageToken: 'page3',
        orderBy: 'status_v2_effective',
      });

      await testLoader.loadNextTestVariants();
      expect(testLoader.failedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([
        variant8,
        variant9,
        variant10,
        variant11,
        variant12,
      ]);
      expect(testLoader.unfilteredFailedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredExecutionErroredVariantsCount).toStrictEqual(
        1,
      );
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(LoadingStage.Done);
      expect(stub.mock.calls.length).toStrictEqual(4);
      expect(stub.mock.lastCall?.[0]).toEqual({
        ...req,
        pageSize: 10000,
        pageToken: 'page4',
        orderBy: 'status_v2_effective',
      });

      // Should not load when the iterator is exhausted.
      await testLoader.loadNextTestVariants();
      expect(testLoader.failedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([
        variant8,
        variant9,
        variant10,
        variant11,
        variant12,
      ]);
      expect(testLoader.unfilteredFailedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredExecutionErroredVariantsCount).toStrictEqual(
        1,
      );
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(LoadingStage.Done);
      expect(stub.mock.calls.length).toStrictEqual(4);
    });

    test('should handle concurrent loadNextPage calls correctly', async () => {
      expect(testLoader.stage).toStrictEqual(
        LoadingStage.LoadingFailedVerdicts,
      );

      const loadReq1 = testLoader.loadNextTestVariants();
      const loadReq2 = testLoader.loadNextTestVariants();
      const loadReq3 = testLoader.loadNextTestVariants();
      const loadReq4 = testLoader.loadNextTestVariants();
      const loadReq5 = testLoader.loadNextTestVariants();
      expect(testLoader.isLoading).toBeTruthy();
      expect(testLoader.stage).toStrictEqual(
        LoadingStage.LoadingFailedVerdicts,
      );

      await loadReq1;
      expect(testLoader.failedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([]);
      // loadReq2 has not finished loading yet.
      expect(testLoader.isLoading).toBeTruthy();
      expect(testLoader.unfilteredFailedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredExecutionErroredVariantsCount).toStrictEqual(
        1,
      );
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(1);
      expect(testLoader.stage).toStrictEqual(LoadingStage.LoadingFlakyVerdicts);

      await loadReq2;
      expect(testLoader.failedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([]);
      // loadReq3 has not finished loading yet.
      expect(testLoader.isLoading).toBeTruthy();
      expect(testLoader.unfilteredFailedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredExecutionErroredVariantsCount).toStrictEqual(
        1,
      );
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(
        LoadingStage.LoadingPassedAndSkippedVerdicts,
      );

      await loadReq3;
      expect(testLoader.failedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([
        variant8,
        variant9,
        variant10,
        variant11,
      ]);
      // loadReq4 has not finished loading yet.
      expect(testLoader.isLoading).toBeTruthy();
      expect(testLoader.unfilteredFailedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredExecutionErroredVariantsCount).toStrictEqual(
        1,
      );
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(
        LoadingStage.LoadingPassedAndSkippedVerdicts,
      );

      await loadReq4;
      expect(testLoader.failedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([
        variant8,
        variant9,
        variant10,
        variant11,
        variant12,
      ]);
      // The list is exhausted, loadReq5 should not change the loading state.
      expect(testLoader.isLoading).toBeFalsy();
      expect(testLoader.unfilteredFailedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredExecutionErroredVariantsCount).toStrictEqual(
        1,
      );
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(LoadingStage.Done);

      await loadReq5;
      expect(testLoader.failedTestVariants).toEqual([variant1, variant2]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([
        variant8,
        variant9,
        variant10,
        variant11,
        variant12,
      ]);
      expect(testLoader.isLoading).toBeFalsy();
      expect(testLoader.unfilteredFailedVariantsCount).toStrictEqual(2);
      expect(testLoader.unfilteredExecutionErroredVariantsCount).toStrictEqual(
        1,
      );
      expect(testLoader.unfilteredFlakyVariantsCount).toStrictEqual(2);
      expect(testLoader.stage).toStrictEqual(LoadingStage.Done);

      expect(stub.mock.calls.length).toStrictEqual(4);
      expect(stub.mock.calls[0][0]).toEqual({
        ...req,
        pageSize: 10000,
        pageToken: '',
        orderBy: 'status_v2_effective',
      });
      expect(stub.mock.calls[1][0]).toEqual({
        ...req,
        pageSize: 10000,
        pageToken: 'page2',
        orderBy: 'status_v2_effective',
      });
      expect(stub.mock.calls[2][0]).toEqual({
        ...req,
        pageSize: 10000,
        pageToken: 'page3',
        orderBy: 'status_v2_effective',
      });
      expect(stub.mock.calls[3][0]).toEqual({
        ...req,
        pageSize: 10000,
        pageToken: 'page4',
        orderBy: 'status_v2_effective',
      });
    });

    test('loadFirstPageOfTestVariants should work correctly', async () => {
      const firstLoadPromise = testLoader.loadNextTestVariants();
      expect(firstLoadPromise).toStrictEqual(
        testLoader.loadFirstPageOfTestVariants(),
      );
      const secondLoadPromise = testLoader.loadNextTestVariants();
      expect(secondLoadPromise).not.toBe(
        testLoader.loadFirstPageOfTestVariants(),
      );
    });

    test('should load at least one test variant that matches the filter', async () => {
      testLoader.filter = (v) => v.testId === 'matched-id';
      await testLoader.loadNextTestVariants();

      expect(testLoader.failedTestVariants).toEqual([]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([variant5]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([]);

      await testLoader.loadNextTestVariants();

      expect(testLoader.failedTestVariants).toEqual([]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([variant5]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([variant12]);
    });

    test('should stop loading at the final page when no test variants matches the filter', async () => {
      testLoader.filter = () => false;

      // Detect infinite loop and abort.
      const oldLoadNextTestVariants =
        testLoader.loadNextTestVariants.bind(testLoader);
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
    let stub: jest.MockedFunction<
      (
        req: QueryTestVariantsRequest,
        cacheOpt?: CacheOption,
      ) => Promise<QueryTestVariantsResponse>
    >;
    const req = { invocations: ['invocation'], pageSize: 4 };

    beforeEach(() => {
      stub = jest.fn();
      stub.mockResolvedValueOnce({ nextPageToken: 'page2' });
      stub.mockResolvedValueOnce({
        testVariants: [variant8],
        nextPageToken: 'page3',
      });
      stub.mockResolvedValueOnce({
        testVariants: [variant9],
        nextPageToken: undefined,
      });
      testLoader = new TestLoader(req, {
        queryTestVariants: stub,
      } as Partial<ResultDb> as ResultDb);
    });

    test('should correctly handle a response with 0 variants', async () => {
      expect(stub.mock.calls.length).toStrictEqual(0);

      await testLoader.loadNextTestVariants();
      expect(testLoader.failedTestVariants).toEqual([]);
      expect(testLoader.nonPassedNorSkippedTestVariants).toEqual([]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([variant8]);
      expect(testLoader.stage).toStrictEqual(
        LoadingStage.LoadingPassedAndSkippedVerdicts,
      );
    });
  });

  describe('when grouping test variants', () => {
    let testLoader: TestLoader;
    let stub: jest.MockedFunction<
      (
        req: QueryTestVariantsRequest,
        cacheOpt?: CacheOption,
      ) => Promise<QueryTestVariantsResponse>
    >;
    const req = { invocations: ['invocation'], pageSize: 4 };

    beforeEach(() => {
      stub = jest.fn();
      stub.mockResolvedValueOnce({
        testVariants: [
          variant1,
          variant2,
          variant3,
          variant4,
          variant5,
          variant6,
          variant7,
        ],
        nextPageToken: 'page1',
      });
      stub.mockResolvedValueOnce({
        testVariants: [variant8, variant9, variant10, variant11, variant12],
      });
      testLoader = new TestLoader(req, {
        queryTestVariants: stub,
      } as Partial<ResultDb> as ResultDb);
    });

    test('should not return empty groups', async () => {
      // [] means there are no groups. [[]] means there is one empty group.
      expect(testLoader.groupedNonPassedNorSkippedVariants).toEqual([]);
    });

    test('should group test variants correctly', async () => {
      testLoader.groupers = [['status', createTVPropGetter('status')]];
      await testLoader.loadNextTestVariants();
      await testLoader.loadNextTestVariants();

      expect(testLoader.groupedNonPassedNorSkippedVariants).toEqual([
        [variant1, variant2],
        [variant3],
        [variant4, variant5],
        [variant6, variant7],
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([
        variant8,
        variant9,
        variant10,
        variant11,
        variant12,
      ]);
    });

    test('should support multiple grouping keys', async () => {
      testLoader.groupers = [
        ['status', createTVPropGetter('status')],
        ['v.key1', createTVPropGetter('v.key1')],
      ];
      await testLoader.loadNextTestVariants();
      await testLoader.loadNextTestVariants();

      expect(testLoader.groupedNonPassedNorSkippedVariants).toEqual([
        [variant1],
        [variant2],
        [variant3],
        [variant4, variant5],
        [variant6],
        [variant7],
      ]);
      expect(testLoader.passedAndSkippedTestVariants).toEqual([
        variant8,
        variant9,
        variant10,
        variant11,
        variant12,
      ]);
    });
  });
});
