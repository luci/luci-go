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

import { assert } from 'chai';
import chai from 'chai';
import sinon from 'sinon';

import '../libs/extensions';
import { chaiRecursiveDeepInclude } from '../libs/test_utils/chai_recursive_deep_include';
import { QueryTestExonerationsRequest, QueryTestResultsRequest, ResultDb, TestExoneration, TestResult } from '../services/resultdb';
import { streamTestBatches, streamTestExonerationBatches, streamTestResultBatches, streamVariantBatches, TestLoader } from './test_loader';
import { ReadonlyTest, ReadonlyVariant, TestNode, VariantStatus } from './test_node';


chai.use(chaiRecursiveDeepInclude);

describe('test_loader', () => {
  const testResult1 = {testId: 'a', resultId: '1', variant: {def: {'key1': 'val1'}}, expected: true} as Partial<TestResult> as TestResult;
  // Result with the same test ID and the same variant.
  const testResult2 = {testId: 'a', resultId: '2', variant: {def: {'key1': 'val1'}}, expected: true} as Partial<TestResult> as TestResult;
  // Result with the same test ID and a different variant.
  const testResult3 = {testId: 'a', resultId: '3', variant: {def: {'key1': 'val2'}}, expected: false} as Partial<TestResult> as TestResult;

  // Result with a different test ID and the same variant.
  const testResult4 = {testId: 'b', resultId: '1', variant: {def: {'key1': 'val2'}}, expected: false} as Partial<TestResult> as TestResult;
  // Result with multiple variant keys.
  const testResult5 = {testId: 'c', resultId: '3', variant: {def: {'key2': 'val1', 'key1': 'val2'}}, expected: true} as Partial<TestResult> as TestResult;
  // Result with the same variant but variant keys are in different order.
  const testResult6 = {testId: 'c', resultId: '4', variant: {def: {'key1': 'val2', 'key2': 'val1'}}, expected: false} as Partial<TestResult> as TestResult;
  // Result with partially different variant.
  const testResult7 = {testId: 'c', resultId: '4', variant: {def: {'key1': 'val2', 'key2': 'val2'}}, expected: true} as Partial<TestResult> as TestResult;
  // Result with an out of order test ID.
  const testResult8 = {testId: 'b', resultId: '2', variant: {def: {'key1': 'val2'}}, expected: false} as Partial<TestResult> as TestResult;

  // Exoneration that shares the same ID and variant with a result.
  // TODO(weiweilin): is this possible?
  const testExoneration1 = {testId: 'a', exonerationId: '1', variant: {def: {'key1': 'val1'}}} as Partial<TestExoneration> as TestExoneration;
  // Exoneration that shares the same ID but a different variant with a result.
  const testExoneration2 = {testId: 'a', exonerationId: '2', variant: {def: {'key1': 'val3'}}} as Partial<TestExoneration> as TestExoneration;
  // Exoneration that has a different testId but shares the same variant with a result.
  const testExoneration3 = {testId: 'd', exonerationId: '1', variant: {def: {'key1': 'val1'}}} as Partial<TestExoneration> as TestExoneration;
  // Exoneration that shares the same ID but a different variant with an exoneration.
  const testExoneration4 = {testId: 'd', exonerationId: '2', variant: {def: {'key1': 'val2'}}} as Partial<TestExoneration> as TestExoneration;
  // Exoneration that has a different ID but shared the same variant with an exoneration.
  const testExoneration5 = {testId: 'e', exonerationId: '1', variant: {def: {'key1': 'val2'}}} as Partial<TestExoneration> as TestExoneration;

  const variant1 = {
    testId: 'a',
    variant: {'def': {'key1': 'val1'}},
    variantKey: 'key1:val1',
    status: VariantStatus.Exonerated,
    results: [testResult1, testResult2],
    exonerations: [testExoneration1],
  };

  const variant2 = {
    testId: 'a',
    variant: {def: {'key1': 'val2'}},
    variantKey: 'key1:val2',
    status: VariantStatus.Unexpected,
    results: [testResult3],
    exonerations: [],
  };

  const variant3 = {
    testId: 'a',
    variant: {def: {'key1': 'val3'}},
    variantKey: 'key1:val3',
    status: VariantStatus.Exonerated,
    results: [],
    exonerations: [testExoneration2],
  };

  const variant4 = {
    testId: 'b',
    variant: {'def': {'key1': 'val2'}},
    variantKey: 'key1:val2',
    status: VariantStatus.Unexpected,
    results: [testResult4, testResult8],
    exonerations: [],
  };

  const variant5 = {
    testId: 'c',
    variant: {def: {'key1': 'val2', 'key2': 'val1'}},
    variantKey: 'key1:val2|key2:val1',
    status: VariantStatus.Flaky,
    results: [testResult5, testResult6],
    exonerations: [],
  };


  const variant6 = {
    testId: 'c',
    variant: {def: {'key1': 'val2', 'key2': 'val2'}},
    variantKey: 'key1:val2|key2:val2',
    status: VariantStatus.Expected,
    results: [testResult7],
    exonerations: [],
  };

  const variant7 = {
    testId: 'd',
    variant: {def: {'key1': 'val1'}},
    variantKey: 'key1:val1',
    status: VariantStatus.Exonerated,
    results: [],
    exonerations: [testExoneration3],
  };

  const variant8 = {
    testId: 'd',
    variant: {def: {'key1': 'val2'}},
    variantKey: 'key1:val2',
    status: VariantStatus.Exonerated,
    results: [],
    exonerations: [testExoneration4],
  };

  const variant9 = {
    testId: 'e',
    variant: {def: {'key1': 'val2'}},
    variantKey: 'key1:val2',
    status: VariantStatus.Exonerated,
    results: [],
    exonerations: [testExoneration5],
  };

  const test1 = {
    id: 'a',
    variants: [
      variant1,
      variant2,
      variant3,
    ],
  };

  const test2 = {
    id: 'b',
    variants: [
      variant4,
    ],
  };

  const test3 = {
    id: 'c',
    variants: [
      variant5,
      variant6,
    ],
  };

  const test4 = {
    id: 'd',
    variants: [
      variant7,
      variant8,
    ],
  };

  const test5 = {
    id: 'e',
    variants: [
      variant9,
    ],
  };

  describe('TestLoader', () => {
    let addTestSpy = sinon.spy();
    let testLoader: TestLoader;
    beforeEach(() => {
      addTestSpy = sinon.spy();
      testLoader = new TestLoader(
        {addTest: addTestSpy} as Partial<TestNode> as TestNode,
        (async function*() {
          yield [test1, test2];
          yield [test3, test4];
          yield [test5];
        })(),
      );
    });

    it('should load tests to test node on request', async () => {
      assert.strictEqual(addTestSpy.callCount, 0);
      await testLoader.loadNextPage();
      assert.strictEqual(addTestSpy.callCount, 2);

      assert.strictEqual(addTestSpy.getCall(0).args[0], test1);
      assert.strictEqual(addTestSpy.getCall(1).args[0], test2);
    });

    it('should preserve loading progress', async () => {
      assert.isFalse(testLoader.done);

      await testLoader.loadNextPage();
      assert.strictEqual(addTestSpy.callCount, 2);
      assert.strictEqual(addTestSpy.getCall(0).args[0], test1);
      assert.strictEqual(addTestSpy.getCall(1).args[0], test2);
      assert.isFalse(testLoader.done);

      await testLoader.loadNextPage();
      assert.strictEqual(addTestSpy.callCount, 4);
      assert.strictEqual(addTestSpy.getCall(2).args[0], test3);
      assert.strictEqual(addTestSpy.getCall(3).args[0], test4);
      assert.isFalse(testLoader.done);

      await testLoader.loadNextPage();
      assert.strictEqual(addTestSpy.callCount, 5);
      assert.strictEqual(addTestSpy.getCall(4).args[0], test5);
      assert.isTrue(testLoader.done);

      // Should not load when the iterator is exhausted.
      await testLoader.loadNextPage();
      assert.strictEqual(addTestSpy.callCount, 5);
      assert.isTrue(testLoader.done);
    });

    it('should handle concurrent loadNextPage calls correctly', async () => {
      assert.isFalse(testLoader.isLoading);
      const loadReq1 = testLoader.loadNextPage();
      const loadReq2 = testLoader.loadNextPage();
      const loadReq3 = testLoader.loadNextPage();
      const loadReq4 = testLoader.loadNextPage();
      assert.isTrue(testLoader.isLoading);

      await loadReq1;
      assert.strictEqual(addTestSpy.callCount, 2);
      // loadReq2 has not finished loading yet.
      assert.isTrue(testLoader.isLoading);
      assert.isFalse(testLoader.done);

      await loadReq2;
      assert.strictEqual(addTestSpy.callCount, 4);
      // loadReq3 has not finished loading yet.
      assert.isTrue(testLoader.isLoading);
      assert.isFalse(testLoader.done);

      await loadReq3;
      assert.strictEqual(addTestSpy.callCount, 5);
      // The list is exhausted, loadReq4 should not change the loading state.
      assert.isFalse(testLoader.isLoading);
      assert.isTrue(testLoader.done);

      await loadReq4;
      assert.strictEqual(addTestSpy.callCount, 5);
      assert.isFalse(testLoader.isLoading);
      assert.isTrue(testLoader.done);
    });
  });

  describe('streamTestResultBatches', () => {
    it('should stream test results from multiple pages', async () => {
      const req = {invocations: ['invocation']} as Partial<QueryTestResultsRequest> as QueryTestResultsRequest;
      const stub = sinon.stub();
      const res1 = {testResults: [testResult1, testResult2, testResult3, testResult4, testResult5 ], nextPageToken: 'token'};
      const res2 = {testResults: [testResult6, testResult7]};
      stub.onCall(0).resolves(res1);
      stub.onCall(1).resolves(res2);
      const resultDb = {queryTestResults: stub} as Partial<ResultDb> as ResultDb;

      const expectedTestResultBatches = [
        [testResult1, testResult2, testResult3, testResult4, testResult5],
        [testResult6, testResult7],
      ];
      let i = 0;
      for await (const testResultBatch of streamTestResultBatches(req, resultDb)) {
        assert.deepStrictEqual(testResultBatch, expectedTestResultBatches[i]);
        i++;
      }
      assert.strictEqual(i, expectedTestResultBatches.length);

      assert.equal(stub.callCount, 2);
      assert.deepEqual(stub.getCall(0).args[0], {...req, pageToken: undefined});
      assert.deepEqual(stub.getCall(1).args[0], {...req, pageToken: res1.nextPageToken});
    });
  });

  describe('streamTestExoneration', () => {
    it('should stream test exonerations from multiple pages', async () => {
      const req = {invocations: ['invocation']} as Partial<QueryTestExonerationsRequest> as QueryTestExonerationsRequest;
      const stub = sinon.stub();
      const res1 = {testExonerations: [testExoneration1, testExoneration2, testExoneration3], nextPageToken: 'token'};
      const res2 = {testExonerations: [testExoneration4, testExoneration5]};
      stub.onCall(0).resolves(res1);
      stub.onCall(1).resolves(res2);
      const resultDb = {queryTestExonerations: stub} as Partial<ResultDb> as ResultDb;

      const expectedTestExonerationBatches = [
        [testExoneration1, testExoneration2, testExoneration3],
        [testExoneration4, testExoneration5],
      ];
      let i = 0;
      for await (const testExonerationBatch of streamTestExonerationBatches(req, resultDb)) {
        assert.deepStrictEqual(testExonerationBatch, expectedTestExonerationBatches[i]);
        i++;
      }

      assert.equal(stub.callCount, 2);
      assert.deepEqual(stub.getCall(0).args[0], {...req, pageToken: undefined});
      assert.deepEqual(stub.getCall(1).args[0], {...req, pageToken: res1.nextPageToken});
    });
  });

  describe('streamVariantBatches', () => {
    it('can group test results and exonerations into test variants correctly', async () => {
      const resultIter = (async function*() {
        yield [testResult1, testResult2, testResult3];
        yield [testResult4, testResult5, testResult6];
        yield [testResult7, testResult8];
      })();
      const exonerationIter = (async function*() {
        yield [testExoneration1, testExoneration2, testExoneration3];
        yield [testExoneration4, testExoneration5];
      })();
      const expectedTestVariants: ReadonlyVariant[] = [];
      for await (const testVariants of streamVariantBatches(resultIter, exonerationIter)) {
        for (const variant of testVariants) {
          expectedTestVariants.push(variant);
        }
      }

      assert.strictEqual(expectedTestVariants.length, 9);

      // The order doesn't matter.
      expectedTestVariants.sort((v1, v2) => v1.testId.localeCompare(v2.testId));

      // Use recursiveDeepInclude to avoid (nested) private properties in actual
      // causing the test to fail.
      assert.recursiveDeepInclude(expectedTestVariants[0], variant1);
      assert.recursiveDeepInclude(expectedTestVariants[1], variant2);
      assert.recursiveDeepInclude(expectedTestVariants[2], variant3);
      assert.recursiveDeepInclude(expectedTestVariants[3], variant4);
      assert.recursiveDeepInclude(expectedTestVariants[4], variant5);
      assert.recursiveDeepInclude(expectedTestVariants[5], variant6);
      assert.recursiveDeepInclude(expectedTestVariants[6], variant7);
      assert.recursiveDeepInclude(expectedTestVariants[7], variant8);
      assert.recursiveDeepInclude(expectedTestVariants[8], variant9);
    });
  });

  describe('streamTestBatches', () => {
    it('can group test variants into tests correctly', async () => {
      const variantIter = (async function*() {
        yield [variant1, variant2, variant3, variant4, variant5];
        yield [variant6, variant7, variant8, variant9];
      })();
      const expectedTests: ReadonlyTest[] = [];
      for await (const tests of streamTestBatches(variantIter)) {
        for (const test of tests) {
          expectedTests.push(test);
        }
      }

      assert.strictEqual(expectedTests.length, 5);

      // The order doesn't matter.
      expectedTests.sort((v1, v2) => v1.id.localeCompare(v2.id));

      // Use recursiveDeepInclude to avoid (nested) private properties in actual
      // causing the test to fail.
      assert.recursiveDeepInclude(expectedTests[0], test1);
      assert.recursiveDeepInclude(expectedTests[1], test2);
      assert.recursiveDeepInclude(expectedTests[2], test3);
      assert.recursiveDeepInclude(expectedTests[3], test4);
      assert.recursiveDeepInclude(expectedTests[4], test5);
    });
  });
});
