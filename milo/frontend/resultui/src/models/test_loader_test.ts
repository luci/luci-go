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
import { QueryTestExonerationsRequest, QueryTestResultRequest, ResultDb, TestExoneration, TestResult } from '../services/resultdb';
import { streamTestExonerations, streamTestResults, streamTests, TestLoader } from './test_loader';
import { ReadonlyTest, TestNode, VariantStatus } from './test_node';


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

  // Exoneration that shares the same ID and variant with a result.
  // TODO(weiweilin): is this possible?
  const testExoneration1 = {testId: 'a', variant: {def: {'key1': 'val1'}}} as Partial<TestExoneration> as TestExoneration;
  // Exoneration that shares the same ID but a different variant with a result.
  const testExoneration2 = {testId: 'a', variant: {def: {'key1': 'val3'}}} as Partial<TestExoneration> as TestExoneration;
  // Exoneration that has a different testId but shares the same variant with a result.
  const testExoneration3 = {testId: 'd', variant: {def: {'key1': 'val1'}}} as Partial<TestExoneration> as TestExoneration;
  // Exoneration that shares the same ID but a different variant with an exoneration.
  const testExoneration4 = {testId: 'd', variant: {def: {'key1': 'val2'}}} as Partial<TestExoneration> as TestExoneration;
  // Exoneration that has a different ID but shared the same variant with an exoneration.
  const testExoneration5 = {testId: 'e', variant: {def: {'key1': 'val2'}}} as Partial<TestExoneration> as TestExoneration;

  const test1 = {
    id: 'a',
    variants: [
      {
        variant: {'def': {'key1': 'val1'}},
        status: VariantStatus.Expected,
        results: [testResult1, testResult2],
        exonerations: [testExoneration1],
      },
      {
        variant: {def: {'key1': 'val2'}},
        status: VariantStatus.Unexpected,
        results: [testResult3],
        exonerations: [],
      },
      {
        variant: {def: {'key1': 'val3'}},
        status: VariantStatus.Exonerated,
        results: [],
        exonerations: [testExoneration2],
      },
    ],
  };

  const test2 = {
    id: 'b',
    variants: [
      {
        variant: {'def': {'key1': 'val2'}},
        status: VariantStatus.Unexpected,
        results: [testResult4],
        exonerations: [],
      },
    ],
  };

  const test3 = {
    id: 'c',
    variants: [
      {
        variant: {def: {'key1': 'val2', 'key2': 'val1'}},
        status: VariantStatus.Flaky,
        results: [testResult5, testResult6],
        exonerations: [],
      },
      {
        variant: {def: {'key1': 'val2', 'key2': 'val2'}},
        status: VariantStatus.Expected,
        results: [testResult7],
        exonerations: [],
      },
    ],
  };

  const test4 = {
    id: 'd',
    variants: [
      {
        variant: {def: {'key1': 'val1'}},
        status: VariantStatus.Exonerated,
        results: [],
        exonerations: [testExoneration3],
      },
      {
        variant: {def: {'key1': 'val2'}},
        status: VariantStatus.Exonerated,
        results: [],
        exonerations: [testExoneration4],
      },
    ],
  };
  const test5 = {
    id: 'e',
    variants: [
      {
        variant: {def: {'key1': 'val2'}},
        status: VariantStatus.Exonerated,
        results: [],
        exonerations: [testExoneration5],
      },
    ],
  };

  describe('TestLoader', () => {
    let spy = sinon.spy();
    let testLoader: TestLoader;
    beforeEach(() => {
      spy = sinon.spy();
      testLoader = new TestLoader(
        {addTest: spy} as Partial<TestNode> as TestNode,
        (async function* () {
          yield test1;
          yield test2;
          yield test3;
          yield test4;
          yield test5;
        })(),
      );
    });

    it('should load tests to test node on request', async () => {
      assert.strictEqual(spy.callCount, 0);
      await testLoader.loadMore(5);
      assert.strictEqual(spy.callCount, 5);

      assert.strictEqual(spy.getCall(0).args[0], test1);
      assert.strictEqual(spy.getCall(1).args[0], test2);
      assert.strictEqual(spy.getCall(2).args[0], test3);
      assert.strictEqual(spy.getCall(3).args[0], test4);
      assert.strictEqual(spy.getCall(4).args[0], test5);
    });

    it('should preserve loading progress', async () => {
      assert.isFalse(testLoader.done);

      await testLoader.loadMore(2);
      assert.strictEqual(spy.callCount, 2);
      assert.strictEqual(spy.getCall(0).args[0], test1);
      assert.strictEqual(spy.getCall(1).args[0], test2);
      assert.isFalse(testLoader.done);

      await testLoader.loadMore(2);
      assert.strictEqual(spy.callCount, 4);
      assert.strictEqual(spy.getCall(2).args[0], test3);
      assert.strictEqual(spy.getCall(3).args[0], test4);
      assert.isFalse(testLoader.done);

      await testLoader.loadMore(2);
      assert.strictEqual(spy.callCount, 5);
      assert.strictEqual(spy.getCall(4).args[0], test5);
      assert.isTrue(testLoader.done);

      // Should not load when the iterator is exhausted.
      await testLoader.loadMore(2);
      assert.strictEqual(spy.callCount, 5);
      assert.isTrue(testLoader.done);
    });

    it('should handle concurrent loadMore calls correctly', async () => {
      assert.isFalse(testLoader.isLoading);
      const loadReq1 = testLoader.loadMore(3);
      const loadReq2 = testLoader.loadMore(3);
      const loadReq3 = testLoader.loadMore(3);
      assert.isTrue(testLoader.isLoading);

      await loadReq1;
      assert.strictEqual(spy.callCount, 3);
      // loadReq2 has not finished loading yet.
      assert.isTrue(testLoader.isLoading);
      assert.isFalse(testLoader.done);

      await loadReq2;
      assert.strictEqual(spy.callCount, 5);
      // The list is exhausted, loadReq3 should not change the loading state.
      assert.isFalse(testLoader.isLoading);
      assert.isTrue(testLoader.done);

      await loadReq3;
      assert.strictEqual(spy.callCount, 5);
      assert.isFalse(testLoader.isLoading);
      assert.isTrue(testLoader.done);
    });
  });

  describe('streamTestResult', () => {
    it('should stream test results from multiple pages', async () => {
      const req = {invocations: ['invocation']} as Partial<QueryTestResultRequest> as QueryTestResultRequest;
      const stub = sinon.stub();
      const res1 = {testResults: [testResult1, testResult2, testResult3, testResult4, testResult5 ], nextPageToken: 'token'};
      const res2 = {testResults: [testResult6, testResult7]};
      stub.onCall(0).resolves(res1);
      stub.onCall(1).resolves(res2);
      const resultDb = {queryTestResults: stub} as Partial<ResultDb> as ResultDb;

      const expectedTestResults = [testResult1, testResult2, testResult3, testResult4, testResult5, testResult6, testResult7];
      let i = 0;
      for await (const testResult of streamTestResults(req, resultDb)) {
        assert.strictEqual(testResult, expectedTestResults[i]);
        i++;
      }
      assert.strictEqual(i, expectedTestResults.length);

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

      const expectedTestExonerations = [testExoneration1, testExoneration2, testExoneration3, testExoneration4, testExoneration5];
      let i = 0;
      for await (const testExoneration of streamTestExonerations(req, resultDb)) {
        assert.strictEqual(testExoneration, expectedTestExonerations[i]);
        i++;
      }

      assert.equal(stub.callCount, 2);
      assert.deepEqual(stub.getCall(0).args[0], {...req, pageToken: undefined});
      assert.deepEqual(stub.getCall(1).args[0], {...req, pageToken: res1.nextPageToken});
    });
  });

  describe('streamTest', () => {
    it('can group test results and exonerations into tests correctly', async () => {
      const resultIter = (async function*() {
        yield testResult1;
        yield testResult2;
        yield testResult3;
        yield testResult4;
        yield testResult5;
        yield testResult6;
        yield testResult7;
      })();
      const exonerationIter = (async function*() {
        yield testExoneration1;
        yield testExoneration2;
        yield testExoneration3;
        yield testExoneration4;
        yield testExoneration5;
      })();
      const tests: ReadonlyTest[] = [];
      for await (const test of streamTests(resultIter, exonerationIter)) {
        tests.push(test);
      }

      assert.strictEqual(tests.length, 5);

      // Use recursiveDeepInclude to avoid (nested) private properties in actual
      // causing the test to fail.
      assert.recursiveDeepInclude(tests[0], test1);
      assert.recursiveDeepInclude(tests[1], test2);
      assert.recursiveDeepInclude(tests[2], test3);
      assert.recursiveDeepInclude(tests[3], test4);
      assert.recursiveDeepInclude(tests[4], test5);
    });
  });
});
