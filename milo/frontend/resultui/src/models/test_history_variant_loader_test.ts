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
import { TestHistoryVariantLoader } from './test_history_variant_loader';

const variant = { def: { key1: 'val1' } };

function createEntry(timestamp: string, invId: string) {
  return {
    invocationIds: [invId],
    invocationTimestamp: timestamp,
    variant,
    variantHash: 'key1:val1',
    status: TestVariantStatus.UNEXPECTED,
    averageDuration: '1s',
  };
}

const entry1 = createEntry('2021-11-05T00:00:02Z', 'inv1');
const entry2 = createEntry('2021-11-05T00:00:01Z', 'inv1');
const entry3 = createEntry('2021-11-05T00:00:00Z', 'inv1');
const entry4 = createEntry('2021-11-04T00:00:00Z', 'inv1');
const entry5 = createEntry('2021-11-03T00:00:00Z', 'inv1');
const entry6 = createEntry('2021-11-02T00:00:00Z', 'inv1');
const entry7 = createEntry('2021-11-02T00:00:00Z', 'inv2');
const entry8 = createEntry('2021-11-01T00:00:00Z', 'inv1');
const entry9 = createEntry('2021-11-01T00:00:00Z', 'inv2');
const entry10 = createEntry('2021-11-01T00:00:00Z', 'inv3');

describe('TestHistoryVariantLoader', () => {
  it('loadUntil should work correctly', async () => {
    // Set up.
    const stub = sinon.stub<[QueryTestHistoryRequest, CacheOption], Promise<QueryTestHistoryResponse>>();
    stub.onCall(0).resolves({ entries: [entry1, entry2], nextPageToken: 'page2' });
    stub.onCall(1).resolves({ entries: [entry3, entry4], nextPageToken: 'page3' });
    stub.onCall(2).resolves({ entries: [entry5] });
    const thvLoader = new TestHistoryVariantLoader(
      'test-realm',
      'test-id',
      variant,
      (resolve) => resolve.toFormat('yyyy-MM-dd'),
      {
        queryTestHistory: stub,
      } as Partial<TestHistoryService> as TestHistoryService
    );

    // Before loading.
    assert.strictEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-05T00:00:00Z'), true), null);
    assert.strictEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-04T00:00:00Z'), true), null);
    assert.strictEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-03T00:00:00Z'), true), null);

    // Load all entries created on or after 2021-11-05.
    await thvLoader.loadUntil(DateTime.fromISO('2021-11-05T12:00:00Z'));
    // The loader should get the 2nd page because it's unclear that all the
    // entries from 2021-11-05 had been loaded after getting the first page.
    assert.deepEqual(stub.getCalls().length, 2);
    assert.deepIncludeProperties(stub.getCall(0).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      variantPredicate: { equals: variant },
    });
    assert.deepIncludeProperties(stub.getCall(1).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      variantPredicate: { equals: variant },
      pageToken: 'page2',
    });

    // After loading.
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-05T00:00:00Z'), true), [entry1, entry2, entry3]);
    // Not all entries from '2021-11-04' has been loaded. Treat them as unloaded.
    assert.strictEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-04T00:00:00Z'), true), null);
    assert.strictEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-03T00:00:00Z'), true), null);

    // Load again with a earlier timestamp but still within the same bucket
    // after resolving.
    await thvLoader.loadUntil(DateTime.fromISO('2021-11-05T01:00:00Z'));
    // The loader should not load the next date.
    assert.deepEqual(stub.getCalls().length, 2);

    // Load all entries created on or after 2021-11-04.
    await thvLoader.loadUntil(DateTime.fromISO('2021-11-04T00:00:00Z'));
    assert.deepEqual(stub.getCalls().length, 3);
    assert.deepIncludeProperties(stub.getCall(2).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      variantPredicate: { equals: variant },
      pageToken: 'page3',
    });

    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-04T00:00:00Z'), true), [entry4]);
    // We've reached the end of the pages. Treat entries from '2021-11-03' as
    // loaded.
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-03T00:00:00Z'), true), [entry5]);
    // Any past date should be considered loaded and return empty array.
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-10-01T00:00:00Z'), true), []);
  });

  it('populateEntries should work correctly', async () => {
    // Set up.
    const stub = sinon.stub<[QueryTestHistoryRequest, CacheOption], Promise<QueryTestHistoryResponse>>();
    const thvLoader = new TestHistoryVariantLoader(
      'test-realm',
      'test-id',
      variant,
      (resolve) => resolve.toFormat('yyyy-MM-dd'),
      {
        queryTestHistory: stub,
      } as Partial<TestHistoryService> as TestHistoryService
    );

    // Load all entries created on or after 2021-11-05.
    stub.onCall(0).resolves({ entries: [entry1, entry2], nextPageToken: 'page2' });
    stub.onCall(1).resolves({ entries: [entry3, entry4], nextPageToken: 'page3' });
    await thvLoader.loadUntil(DateTime.fromISO('2021-11-05T12:00:00Z'));

    // Populate entries manually.
    const afterTime = DateTime.fromISO(entry6.invocationTimestamp);
    thvLoader.populateEntries([entry4, entry5, entry6], afterTime);

    // Entry 4 should not be duplicated.
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-04T00:00:00Z'), true), [entry4]);
    // Entry 5 should be recorded.
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-03T00:00:00Z'), true), [entry5]);
    // Entry 6 should also be recorded, but we don't know whether all entries on
    // 2021-11-02 has been loaded yet.
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-02T00:00:00Z'), true), null);

    // Load all entries created on or after 2021-11-02.
    stub.onCall(2).resolves({ entries: [entry6, entry7], nextPageToken: 'page2' });
    stub.onCall(3).resolves({ entries: [entry8, entry9], nextPageToken: 'page3' });
    await thvLoader.loadUntil(DateTime.fromISO('2021-11-02T00:00:00Z'));

    // The loader should skip ahead.
    assert.deepEqual(stub.getCalls().length, 4);
    assert.deepIncludeProperties(stub.getCall(2).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      variantPredicate: { equals: variant },
    });
    assert.strictEqual(stub.getCall(2).args[0].timeRange.latest, afterTime.toISO());
    assert.isFalse(Boolean(stub.getCall(2).args[0].pageToken));
    assert.deepIncludeProperties(stub.getCall(3).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      variantPredicate: { equals: variant },
      pageToken: 'page2',
    });
    assert.strictEqual(stub.getCall(3).args[0].timeRange.latest, afterTime.toISO());

    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-02T00:00:00Z'), true), [entry6, entry7]);
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-01T00:00:00Z'), true), null);

    // Populate entries with already loaded entries.
    thvLoader.populateEntries([entry4, entry5, entry6], afterTime);

    // It should not affect any getEntries call.
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-04T00:00:00Z'), true), [entry4]);
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-03T00:00:00Z'), true), [entry5]);
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-02T00:00:00Z'), true), [entry6, entry7]);
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-01T00:00:00Z'), true), null);

    // Load all entries created on or after 2021-11-01.
    stub.onCall(4).resolves({ entries: [entry10] });
    await thvLoader.loadUntil(DateTime.fromISO('2021-11-01T00:00:00Z'));

    // The loader should not skip ahead.
    assert.deepEqual(stub.getCalls().length, 5);
    assert.deepIncludeProperties(stub.getCall(4).args[0], {
      realm: 'test-realm',
      testId: 'test-id',
      variantPredicate: { equals: variant },
      pageToken: 'page3',
    });
    assert.strictEqual(stub.getCall(3).args[0].timeRange.latest, afterTime.toISO());
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-01T00:00:00Z'), true), [entry8, entry9, entry10]);
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-10-01T00:00:00Z'), true), []);
  });
});
