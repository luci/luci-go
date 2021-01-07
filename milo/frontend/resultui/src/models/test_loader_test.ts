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

import { chaiRecursiveDeepInclude } from '../libs/test_utils/chai_recursive_deep_include';
import { QueryTestVariantsRequest, TestVariantStatus, UISpecificService } from '../services/resultdb';
import { streamVariantBatches, TestLoader } from './test_loader';
import { TestNode } from './test_node';


chai.use(chaiRecursiveDeepInclude);

describe('test_loader', () => {
  const variant1 = {
    testId: 'a',
    variant: {'def': {'key1': 'val1'}},
    variantHash: 'key1:val1',
    status: TestVariantStatus.EXONERATED,
  };

  const variant2 = {
    testId: 'a',
    variant: {def: {'key1': 'val2'}},
    variantHash: 'key1:val2',
    status: TestVariantStatus.UNEXPECTED,
  };

  const variant3 = {
    testId: 'a',
    variant: {def: {'key1': 'val3'}},
    variantHash: 'key1:val3',
    status: TestVariantStatus.EXONERATED,
  };

  const variant4 = {
    testId: 'b',
    variant: {'def': {'key1': 'val2'}},
    variantHash: 'key1:val2',
    status: TestVariantStatus.UNEXPECTED,
  };

  const variant5 = {
    testId: 'c',
    variant: {def: {'key1': 'val2', 'key2': 'val1'}},
    variantHash: 'key1:val2|key2:val1',
    status: TestVariantStatus.FLAKY,
  };


  const variant6 = {
    testId: 'c',
    variant: {def: {'key1': 'val2', 'key2': 'val2'}},
    variantHash: 'key1:val2|key2:val2',
    status: TestVariantStatus.EXPECTED,
  };

  const variant7 = {
    testId: 'd',
    variant: {def: {'key1': 'val1'}},
    variantHash: 'key1:val1',
    status: TestVariantStatus.EXONERATED,
  };

  const variant8 = {
    testId: 'd',
    variant: {def: {'key1': 'val2'}},
    variantHash: 'key1:val2',
    status: TestVariantStatus.EXONERATED,
  };

  const variant9 = {
    testId: 'e',
    variant: {def: {'key1': 'val2'}},
    variantHash: 'key1:val2',
    status: TestVariantStatus.EXONERATED,
  };

  describe('TestLoader', () => {
    let addTestSpy = sinon.spy();
    let testLoader: TestLoader;
    beforeEach(() => {
      addTestSpy = sinon.spy();
      testLoader = new TestLoader(
        {addTestId: addTestSpy} as Partial<TestNode> as TestNode,
        (async function*() {
          yield [variant1, variant2, variant3, variant4];
          yield [variant5, variant6, variant7, variant8];
          yield [variant9];
        })(),
      );
    });

    it('should load tests to test node on request', async () => {
      assert.strictEqual(addTestSpy.callCount, 0);
      await testLoader.loadNextPage();
      assert.strictEqual(addTestSpy.callCount, 4);

      assert.strictEqual(addTestSpy.getCall(0).args[0], variant1.testId);
      assert.strictEqual(addTestSpy.getCall(1).args[0], variant2.testId);
      assert.strictEqual(addTestSpy.getCall(2).args[0], variant3.testId);
      assert.strictEqual(addTestSpy.getCall(3).args[0], variant4.testId);
    });

    it('should preserve loading progress', async () => {
      assert.isFalse(testLoader.done);

      await testLoader.loadNextPage();
      assert.strictEqual(addTestSpy.callCount, 4);
      assert.strictEqual(addTestSpy.getCall(0).args[0], variant1.testId);
      assert.strictEqual(addTestSpy.getCall(1).args[0], variant2.testId);
      assert.strictEqual(addTestSpy.getCall(2).args[0], variant3.testId);
      assert.strictEqual(addTestSpy.getCall(3).args[0], variant4.testId);
      assert.isFalse(testLoader.done);

      await testLoader.loadNextPage();
      assert.strictEqual(addTestSpy.callCount, 8);
      assert.strictEqual(addTestSpy.getCall(4).args[0], variant5.testId);
      assert.strictEqual(addTestSpy.getCall(5).args[0], variant6.testId);
      assert.strictEqual(addTestSpy.getCall(6).args[0], variant7.testId);
      assert.strictEqual(addTestSpy.getCall(7).args[0], variant8.testId);
      assert.isFalse(testLoader.done);

      await testLoader.loadNextPage();
      assert.strictEqual(addTestSpy.callCount, 9);
      assert.strictEqual(addTestSpy.getCall(8).args[0], variant9.testId);
      assert.isTrue(testLoader.done);

      // Should not load when the iterator is exhausted.
      await testLoader.loadNextPage();
      assert.strictEqual(addTestSpy.callCount, 9);
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
      assert.strictEqual(addTestSpy.callCount, 4);
      // loadReq2 has not finished loading yet.
      assert.isTrue(testLoader.isLoading);
      assert.isFalse(testLoader.done);

      await loadReq2;
      assert.strictEqual(addTestSpy.callCount, 8);
      // loadReq3 has not finished loading yet.
      assert.isTrue(testLoader.isLoading);
      assert.isFalse(testLoader.done);

      await loadReq3;
      assert.strictEqual(addTestSpy.callCount, 9);
      // The list is exhausted, loadReq4 should not change the loading state.
      assert.isFalse(testLoader.isLoading);
      assert.isTrue(testLoader.done);

      await loadReq4;
      assert.strictEqual(addTestSpy.callCount, 9);
      assert.isFalse(testLoader.isLoading);
      assert.isTrue(testLoader.done);
    });
  });

  describe('streamVariantBatches', () => {
    it('should stream test exonerations from multiple pages', async () => {
      const req = {invocations: ['invocation']} as Partial<QueryTestVariantsRequest> as QueryTestVariantsRequest;
      const stub = sinon.stub();
      const res1 = {testVariants: [variant1, variant2, variant3, variant4, variant5], nextPageToken: 'token'};
      const res2 = {testVariants: [variant6, variant7, variant8, variant9]};
      stub.onCall(0).resolves(res1);
      stub.onCall(1).resolves(res2);
      const resultDb = {queryTestVariants: stub} as Partial<UISpecificService> as UISpecificService;

      const expectedTestVariantBatches = [
        [variant1, variant2, variant3, variant4, variant5],
        [variant6, variant7, variant8, variant9],
      ];
      let i = 0;
      for await (const testVariantBatch of streamVariantBatches(req, resultDb)) {
        assert.deepStrictEqual(testVariantBatch, expectedTestVariantBatches[i]);
        i++;
      }

      assert.equal(stub.callCount, 2);
      assert.deepEqual(stub.getCall(0).args[0], {...req, pageToken: undefined});
      assert.deepEqual(stub.getCall(1).args[0], {...req, pageToken: res1.nextPageToken});
    });
  });
});
