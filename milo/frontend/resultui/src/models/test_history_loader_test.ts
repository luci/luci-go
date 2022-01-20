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
import sinon from 'sinon';

import { CacheOption } from '../libs/cached_fn';
import {
  QueryUniqueTestVariantsRequest,
  QueryUniqueTestVariantsResponse,
  ResultDb,
  UniqueTestVariant,
} from '../services/resultdb';
import {
  QueryTestHistoryRequest,
  QueryTestHistoryResponse,
  TestHistoryService,
} from '../services/test_history_service';
import { TestHistoryLoader } from './test_history_loader';

const utv1: UniqueTestVariant = {
  realm: 'test-realm',
  testId: 'test-id',
  variantHash: 'key1:val1',
  variant: {
    def: {
      key1: 'val1',
    },
  },
};

const utv2: UniqueTestVariant = {
  realm: 'test-realm',
  testId: 'test-id',
  variantHash: 'key1:val2',
  variant: {
    def: {
      key1: 'val2',
    },
  },
};

describe('TestHistoryLoader', () => {
  it('discoverVariants should work correctly', async () => {
    // Set up.
    const queryTestHistoryStub = sinon.stub<
      [QueryTestHistoryRequest, CacheOption],
      Promise<QueryTestHistoryResponse>
    >();
    const queryUniqueTestVariantsStub = sinon.stub<
      [QueryUniqueTestVariantsRequest, CacheOption],
      Promise<QueryUniqueTestVariantsResponse>
    >();
    const testHistoryLoader = new TestHistoryLoader(
      'test-realm',
      'test-id',
      (resolve) => resolve.toFormat('yyyy-MM-dd'),
      {
        queryTestHistory: queryTestHistoryStub,
      } as Partial<TestHistoryService> as TestHistoryService,
      {
        queryUniqueTestVariants: queryUniqueTestVariantsStub,
      } as Partial<ResultDb> as ResultDb
    );

    // Before loading.
    assert.strictEqual(testHistoryLoader.variants.length, 0);

    // Discover variants from the first page.
    queryUniqueTestVariantsStub.onCall(0).resolves({ variants: [utv1], nextPageToken: 'page2' });
    let done = await testHistoryLoader.discoverVariants();
    assert.isFalse(done);
    assert.deepEqual(queryUniqueTestVariantsStub.getCalls().length, 1);
    assert.deepIncludeProperties(queryUniqueTestVariantsStub.getCall(0).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      pageToken: '',
    });

    // After loading.
    assert.deepEqual(testHistoryLoader.variants, [['key1:val1', { def: { key1: 'val1' } }]]);

    // Discover variants from the second page.
    queryUniqueTestVariantsStub.onCall(1).resolves({ variants: [utv2] });
    done = await testHistoryLoader.discoverVariants();
    assert.isTrue(done);
    assert.deepEqual(queryUniqueTestVariantsStub.getCalls().length, 2);
    assert.deepIncludeProperties(queryUniqueTestVariantsStub.getCall(1).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      pageToken: 'page2',
    });

    // After loading.
    assert.deepEqual(testHistoryLoader.variants, [
      ['key1:val1', { def: { key1: 'val1' } }],
      ['key1:val2', { def: { key1: 'val2' } }],
    ]);
  });
});
