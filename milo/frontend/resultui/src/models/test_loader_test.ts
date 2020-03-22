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
import sinon from 'sinon';

import { QueryTestExonerationsRequest, QueryTestResultRequest, ResultDb, TestExoneration, TestResult } from '../services/resultdb';
import { streamTestExonerations, streamTestResultOrExoneration, streamTestResults, streamTests, TestLoader } from './test_loader';
import { ReadonlyTest, TestNode } from './test_node';


describe('test_loader', () => {
  describe('TestLoader', () => {
    it('should load tests to test node on request', async () => {
      const spy = sinon.spy();
      const test1 = {} as Partial<ReadonlyTest> as ReadonlyTest;
      const test2 = {} as Partial<ReadonlyTest> as ReadonlyTest;
      const test3 = {} as Partial<ReadonlyTest> as ReadonlyTest;
      const testLoader = new TestLoader(
        {addTest: spy} as Partial<TestNode> as TestNode,
        (async function* () {
          yield test1;
          yield test2;
          yield test3;
        })(),
      );
      await testLoader.loadMore(2);
      assert.strictEqual(spy.callCount, 2);
      assert.isTrue(spy.getCall(0).calledWith(test1));
      assert.isTrue(spy.getCall(1).calledWith(test2));
    });

    it('should preserve loading progress', async () => {
      const spy = sinon.spy();
      const test1 = {} as Partial<ReadonlyTest> as ReadonlyTest;
      const test2 = {} as Partial<ReadonlyTest> as ReadonlyTest;
      const test3 = {} as Partial<ReadonlyTest> as ReadonlyTest;
      const testLoader = new TestLoader(
        {addTest: spy} as Partial<TestNode> as TestNode,
        (async function* () {
          yield test1;
          yield test2;
          yield test3;
        })(),
      );
      await testLoader.loadMore(1);
      assert.strictEqual(spy.callCount, 1);
      assert.isTrue(spy.getCall(0).calledWith(test1));

      await testLoader.loadMore(1);
      assert.strictEqual(spy.callCount, 2);
      assert.isTrue(spy.getCall(1).calledWith(test2));
    });

    it('should not load when the iterator is exhausted', async () => {
      const spy = sinon.spy();
      const test1 = {} as Partial<ReadonlyTest> as ReadonlyTest;
      const test2 = {} as Partial<ReadonlyTest> as ReadonlyTest;
      const test3 = {} as Partial<ReadonlyTest> as ReadonlyTest;
      const testLoader = new TestLoader(
        {addTest: spy} as Partial<TestNode> as TestNode,
        (async function* () {
          yield test1;
          yield test2;
          yield test3;
        })(),
      );
      await testLoader.loadMore(4);
      assert.strictEqual(spy.callCount, 3);
      await testLoader.loadMore(4);
      assert.strictEqual(spy.callCount, 3);
    });
  });

  describe('streamTestResult', () => {
    it('should stream test results from multiple pages', async () => {
      const req = {invocations: ['invocation']} as Partial<QueryTestResultRequest> as QueryTestResultRequest;
      const stub = sinon.stub();
      const testResult1 = {} as Partial<TestResult> as TestResult;
      const testResult2 = {} as Partial<TestResult> as TestResult;
      const testResult3 = {} as Partial<TestResult> as TestResult;
      const testResults = [testResult1, testResult2, testResult3];
      const res1 = {testResults: [testResult1, testResult2], nextPageToken: 'token'};
      const res2 = {testResults: [testResult3]};
      stub.onCall(0).resolves(res1);
      stub.onCall(1).resolves(res2);
      stub.returnValues = [Promise.resolve(res1), Promise.resolve(res2)];
      const resultDb = {queryTestResults: stub} as Partial<ResultDb> as ResultDb;

      let i = 0;
      for await (const testResult of streamTestResults(req, resultDb)) {
        assert.strictEqual(testResult, testResults[i]);
        i++;
      }
      assert.strictEqual(i, testResults.length);

      assert.equal(stub.callCount, 2);
      assert.deepEqual(stub.getCall(0).args[0], {...req, pageToken: undefined});
      assert.deepEqual(stub.getCall(1).args[0], {...req, pageToken: res1.nextPageToken});
    });
  });

  describe('streamTestExoneration', () => {
    it('should stream test exonerations from multiple pages', async () => {
      const req = {invocations: ['invocation']} as Partial<QueryTestExonerationsRequest> as QueryTestExonerationsRequest;
      const stub = sinon.stub();
      const testExoneration1 = {} as Partial<TestExoneration> as TestExoneration;
      const testExoneration2 = {} as Partial<TestExoneration> as TestExoneration;
      const testExoneration3 = {} as Partial<TestExoneration> as TestExoneration;
      const testExonerations = [testExoneration1, testExoneration2, testExoneration3];
      const res1 = {testExonerations: [testExoneration1, testExoneration2], nextPageToken: 'token'};
      const res2 = {testExonerations: [testExoneration3]};
      stub.onCall(0).resolves(res1);
      stub.onCall(1).resolves(res2);
      stub.returnValues = [Promise.resolve(res1), Promise.resolve(res2)];
      const resultDb = {queryTestExonerations: stub} as Partial<ResultDb> as ResultDb;
      let i = 0;
      for await (const testExoneration of streamTestExonerations(req, resultDb)) {
        assert.strictEqual(testExoneration, testExonerations[i]);
        i++;
      }

      assert.equal(stub.callCount, 2);
      assert.deepEqual(stub.getCall(0).args[0], {...req, pageToken: undefined});
      assert.deepEqual(stub.getCall(1).args[0], {...req, pageToken: res1.nextPageToken});
    });
  });

  describe('streamTestObject', () => {
    it('should merge result and exoneration streams sorted by test ID', async () => {
      const testResult1 = {testId: 'a'} as Partial<TestResult> as TestResult;
      const testResult2 = {testId: 'c'} as Partial<TestResult> as TestResult;
      const testResult3 = {testId: 'f'} as Partial<TestResult> as TestResult;
      const testExoneration1 = {testId: 'b'} as Partial<TestExoneration> as TestExoneration;
      const testExoneration2 = {testId: 'c'} as Partial<TestExoneration> as TestExoneration;
      const testExoneration3 = {testId: 'd'} as Partial<TestExoneration> as TestExoneration;
      const testObjects = [
        testResult1,
        testExoneration1,
        testResult2,
        // when testId is the same, exonerations should come after results.
        testExoneration2,
        testExoneration3,
        testResult3,
      ];
      const tesultIter = (async function*() {
        yield testResult1;
        yield testResult2;
        yield testResult3;
      })();
      const exonerationIter = (async function*() {
        yield testExoneration1;
        yield testExoneration2;
        yield testExoneration3;
      })();

      let i = 0;
      for await (const testObject of streamTestResultOrExoneration(tesultIter, exonerationIter)) {
        assert.strictEqual(testObject, testObjects[i]);
        i++;
      }
      assert.strictEqual(i, testObjects.length);
    });
  });

  describe('streamTest', () => {
    it('can stream tests from test objects', async () => {
      const testResult1 = {testId: 'a', resultId: '1', variant: {def: {'key': 'val1'}}} as Partial<TestResult> as TestResult;
      const testResult2 = {testId: 'c', resultId: '2', variant: {def: {'key': 'val1'}}} as Partial<TestResult> as TestResult;
      const testResult3 = {testId: 'c', resultId: '3', variant: {def: {'key': 'val2'}}} as Partial<TestResult> as TestResult;
      const testResult4 = {testId: 'd', resultId: '4', variant: {def: {'key': 'val2'}}} as Partial<TestResult> as TestResult;
      const testExoneration1 = {testId: 'a', variant: {def: {'key': 'val1'}}} as Partial<TestExoneration> as TestExoneration;
      const testExoneration2 = {testId: 'a', variant: {def: {'key': 'val2'}}} as Partial<TestExoneration> as TestExoneration;
      const testExoneration3 = {testId: 'd', variant: {def: {'key': 'val1'}}} as Partial<TestExoneration> as TestExoneration;
      const testObjIter = (async function*() {
        yield testResult1;
        yield testExoneration1;
        yield testExoneration2;
        yield testResult2;
        yield testResult3;
        yield testResult4;
        yield testExoneration3;
      })();

      const tests: ReadonlyTest[] = [];
      for await (const test of streamTests(testObjIter)) {
        tests.push(test);
      }

      assert.strictEqual(tests.length, 3);
      assert.deepNestedInclude(tests[0], {id: 'a'});
      assert.strictEqual(tests[0].variants.length, 2);
      assert.deepNestedInclude(
        tests[0].variants[0], 
        {
          variant: {def: {'key': 'val1'}},
          results: [testResult1],
          exonerations: [testExoneration1],
        },
      );
      assert.deepNestedInclude(
        tests[0].variants[1], 
        {
          variant: {def: {'key': 'val2'}},
          results: [],
          exonerations: [testExoneration2],
        },
      );

      assert.deepNestedInclude(tests[1], {id: 'c'});
      assert.strictEqual(tests[1].variants.length, 2);
      assert.deepNestedInclude(
        tests[1].variants[0], 
        {
          variant: {def: {'key': 'val1'}},
          results: [testResult2],
          exonerations: [],
        },
      );
      assert.deepNestedInclude(
        tests[1].variants[1], 
        {
          variant: {def: {'key': 'val2'}},
          results: [testResult3],
          exonerations: [],
        },
      );

      assert.deepNestedInclude(tests[2], {id: 'd'});
      assert.strictEqual(tests[1].variants.length, 2);
      assert.deepNestedInclude(
        tests[2].variants[0], 
        {
          variant: {def: {'key': 'val2'}},
          results: [testResult4],
          exonerations: [],
        },
      );
      assert.deepNestedInclude(
        tests[2].variants[1], 
        {
          variant: {def: {'key': 'val1'}},
          results: [],
          exonerations: [testExoneration3],
        },
      );
    });
  });
});
