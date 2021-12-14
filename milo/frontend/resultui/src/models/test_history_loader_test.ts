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
import { DateTime } from 'luxon';
import sinon from 'sinon';

import { CacheOption } from '../libs/cached_fn';
import { TestVariantStatus } from '../services/resultdb';
import {
  QueryTestHistoryRequest,
  QueryTestHistoryResponse,
  TestHistoryService,
} from '../services/test_history_service';
import { TestHistoryLoader } from './test_history_loader';

const entry1 = {
  invocationIds: ['inv'],
  invocationTimestamp: '2021-11-05T00:00:01Z',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
  averageDuration: '1s',
};

const entry2 = {
  invocationIds: ['inv'],
  invocationTimestamp: '2021-11-05T00:00:00Z',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
  averageDuration: '1s',
};

const entry3 = {
  invocationIds: ['inv'],
  invocationTimestamp: '2021-11-05T00:00:00Z',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.UNEXPECTED,
  averageDuration: '1s',
};

const entry4 = {
  invocationIds: ['inv'],
  invocationTimestamp: '2021-11-04T00:00:00Z',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
  averageDuration: '1s',
};

const entry5 = {
  invocationIds: ['inv'],
  invocationTimestamp: '2021-11-03T00:00:00Z',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
  averageDuration: '1s',
};

describe('TestHistoryLoader', () => {
  it('discoverVariant should work correctly', async () => {
    // Set up.
    const stub = sinon.stub<[QueryTestHistoryRequest, CacheOption], Promise<QueryTestHistoryResponse>>();
    const testHistoryLoader = new TestHistoryLoader(
      'test-realm',
      'test-id',
      (resolve) => resolve.toFormat('yyyy-MM-dd'),
      {
        queryTestHistory: stub,
      } as Partial<TestHistoryService> as TestHistoryService
    );

    // Before loading.
    assert.strictEqual(testHistoryLoader.variants.length, 0);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-05'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-05'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-04'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-04'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-03'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-03'), true), null);

    // Discover variants from the first page.
    stub.onCall(0).resolves({ entries: [entry1, entry2], nextPageToken: 'page2' });
    let done = await testHistoryLoader.discoverVariants(undefined);
    assert.isFalse(done);
    assert.deepEqual(stub.getCalls().length, 1);
    assert.deepIncludeProperties(stub.getCall(0).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      pageToken: '',
    });

    // After loading.
    assert.deepEqual(testHistoryLoader.variants, [['key1:val1', { def: { key1: 'val1' } }]]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-05'), true), null);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-05'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-04'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-04'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-03'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-03'), true), null);

    // Discover variants from the second page.
    stub.onCall(1).resolves({ entries: [entry3, entry4], nextPageToken: 'page3' });
    done = await testHistoryLoader.discoverVariants(undefined);
    assert.isFalse(done);
    assert.deepEqual(stub.getCalls().length, 2);
    assert.deepIncludeProperties(stub.getCall(1).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      pageToken: 'page2',
    });

    // After loading.
    assert.deepEqual(testHistoryLoader.variants, [
      ['key1:val1', { def: { key1: 'val1' } }],
      ['key1:val2', { def: { key1: 'val2' } }],
    ]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-05'), true), [entry1, entry2]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-05'), true), [entry3]);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-04'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-04'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-03'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-03'), true), null);

    // Discover variants from the last page.
    stub.onCall(2).resolves({ entries: [entry5] });
    done = await testHistoryLoader.discoverVariants(undefined);
    assert.isTrue(done);
    assert.deepEqual(stub.getCalls().length, 3);
    assert.deepIncludeProperties(stub.getCall(2).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      pageToken: 'page3',
    });

    // After loading.
    assert.deepEqual(testHistoryLoader.variants, [
      ['key1:val1', { def: { key1: 'val1' } }],
      ['key1:val2', { def: { key1: 'val2' } }],
    ]);
    // All variants should be considered finalized since we have reached the end of the page.
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-05'), true), [entry1, entry2]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-05'), true), [entry3]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-04'), true), [entry4]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-04'), true), []);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-03'), true), [entry5]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-03'), true), []);
  });

  it('discoverVariant should work correctly with predicates', async () => {
    // Set up.
    const stub = sinon.stub<[QueryTestHistoryRequest, CacheOption], Promise<QueryTestHistoryResponse>>();
    const testHistoryLoader = new TestHistoryLoader(
      'test-realm',
      'test-id',
      (resolve) => resolve.toFormat('yyyy-MM-dd'),
      {
        queryTestHistory: stub,
      } as Partial<TestHistoryService> as TestHistoryService
    );
    const predicate1 = { contains: { def: { key1: 'val1' } } };
    const predicate2 = { contains: { def: { key1: 'val2' } } };

    // Before loading.
    assert.strictEqual(testHistoryLoader.variants.length, 0);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-05'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-05'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-04'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-04'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-03'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-03'), true), null);

    // Discover variants from the first page.
    stub.onCall(0).resolves({ entries: [entry1, entry2], nextPageToken: 'page2' });
    let done = await testHistoryLoader.discoverVariants(predicate1);
    assert.isFalse(done);
    assert.deepEqual(stub.getCalls().length, 1);
    assert.deepIncludeProperties(stub.getCall(0).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      variantPredicate: predicate1,
      pageToken: '',
    });

    // After loading.
    assert.deepEqual(testHistoryLoader.variants, [['key1:val1', { def: { key1: 'val1' } }]]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-05'), true), null);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-05'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-04'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-04'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-03'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-03'), true), null);

    // Discover variants from the second page with the same predicate.
    stub.onCall(1).resolves({ entries: [entry4] });
    done = await testHistoryLoader.discoverVariants(predicate1);
    assert.isTrue(done);
    assert.deepEqual(stub.getCalls().length, 2);
    assert.deepIncludeProperties(stub.getCall(1).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      variantPredicate: predicate1,
      pageToken: 'page2',
    });

    // After loading. Only 'key1:val1' variant should be considered finalized.
    assert.deepEqual(testHistoryLoader.variants, [['key1:val1', { def: { key1: 'val1' } }]]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-05'), true), [entry1, entry2]);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-05'), true), null);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-04'), true), [entry4]);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-04'), true), null);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-03'), true), []);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-03'), true), null);

    // Discover variants with a different predicate.
    stub.onCall(2).resolves({ entries: [entry3] });
    done = await testHistoryLoader.discoverVariants(predicate2);
    assert.isTrue(done);
    assert.deepEqual(stub.getCalls().length, 3);
    assert.deepIncludeProperties(stub.getCall(2).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      variantPredicate: predicate2,
      pageToken: '',
    });

    // After loading.
    assert.deepEqual(testHistoryLoader.variants, [
      ['key1:val1', { def: { key1: 'val1' } }],
      ['key1:val2', { def: { key1: 'val2' } }],
    ]);
    // All variants should be considered finalized since we have reached the end of the page.
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-05'), true), [entry1, entry2]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-05'), true), [entry3]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-04'), true), [entry4]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-04'), true), []);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-03'), true), []);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-03'), true), []);
  });
});
