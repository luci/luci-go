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
  invocationTimestamp: '2021-11-05T00:00:01Z',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
  averageDuration: '1s',
};

const entry2 = {
  invocationTimestamp: '2021-11-05T00:00:00Z',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.UNEXPECTED,
  averageDuration: '1s',
};

const entry3 = {
  invocationTimestamp: '2021-11-05T00:00:00Z',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
  averageDuration: '1s',
};

const entry4 = {
  invocationTimestamp: '2021-11-04T00:00:00Z',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
  averageDuration: '1s',
};

const entry5 = {
  invocationTimestamp: '2021-11-03T00:00:00Z',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
  averageDuration: '1s',
};

describe('TestHistoryLoader', () => {
  let testHistoryLoader: TestHistoryLoader;
  let stub = sinon.stub<[QueryTestHistoryRequest, CacheOption], Promise<QueryTestHistoryResponse>>();

  beforeEach(() => {
    stub = sinon.stub();
    stub.onCall(0).resolves({ entries: [entry1, entry2], nextPageToken: 'page2' });
    stub.onCall(1).resolves({ entries: [entry3, entry4], nextPageToken: 'page3' });
    stub.onCall(2).resolves({ entries: [entry5] });
    testHistoryLoader = new TestHistoryLoader('test-realm', 'test-id', (resolve) => resolve.toFormat('yyyy-MM-dd'), {
      queryTestHistory: stub,
    } as Partial<TestHistoryService> as TestHistoryService);
  });

  it('loadUntil should work correctly', async () => {
    // Before loading.
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-05T00:00:00Z'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-05T00:00:00Z'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-04T00:00:00Z'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-04T00:00:00Z'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-03T00:00:00Z'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-03T00:00:00Z'), true), null);

    // Load all entries created on or after 2021-11-05.
    await testHistoryLoader.loadUntil(DateTime.fromISO('2021-11-05T00:00:00Z'));
    // The loader should get the 2nd page because it's unclear that all the
    // entries from 2021-11-05 had been loaded after getting the first page.
    assert.deepEqual(stub.getCalls().length, 2);
    assert.deepIncludeProperties(stub.getCall(0).args[0], { realm: 'test-realm', testId: 'test-id' });
    assert.deepIncludeProperties(stub.getCall(1).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      pageToken: 'page2',
    });

    // After loading.
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-05T00:00:00Z'), true), [
      entry1,
      entry3,
    ]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-05T00:00:00Z'), true), [
      entry2,
    ]);
    // Not all entries from '2021-11-04' has been loaded. Treat them as unloaded.
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-04T00:00:00Z'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-04T00:00:00Z'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-03T00:00:00Z'), true), null);
    assert.strictEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-03T00:00:00Z'), true), null);

    // Load all entries created on or after 2021-11-04.
    await testHistoryLoader.loadUntil(DateTime.fromISO('2021-11-04T00:00:00Z'));
    assert.deepEqual(stub.getCalls().length, 3);
    assert.deepIncludeProperties(stub.getCall(2).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      pageToken: 'page3',
    });

    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-04T00:00:00Z'), true), [
      entry4,
    ]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-04T00:00:00Z'), true), []);
    // We've reached the end of the pages. Treat entries from '2021-11-03' as
    // loaded.
    assert.deepEqual(testHistoryLoader.getEntries('key1:val1', DateTime.fromISO('2021-11-03T00:00:00Z'), true), [
      entry5,
    ]);
    assert.deepEqual(testHistoryLoader.getEntries('key1:val2', DateTime.fromISO('2021-11-03T00:00:00Z'), true), []);
  });
});
