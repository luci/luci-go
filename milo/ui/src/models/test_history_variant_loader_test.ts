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

import { assert, expect } from 'chai';
import chaiSubset from 'chai-subset';
import { DateTime } from 'luxon';
import sinon from 'sinon';

import { CacheOption } from '../libs/cached_fn';
import {
  QueryTestHistoryRequest,
  QueryTestHistoryResponse,
  TestHistoryService,
  TestVerdictStatus,
} from '../services/luci_analysis';
import { TestHistoryVariantLoader } from './test_history_variant_loader';

chai.use(chaiSubset);

const variant = { def: { key1: 'val1' } };

function createEntry(timestamp: string, invId: string) {
  return {
    testId: 'test',
    invocationId: invId,
    partitionTime: timestamp,
    variant,
    variantHash: 'key1:val1',
    status: TestVerdictStatus.UNEXPECTED,
    passedAvgDuration: '1s',
  };
}

const entry1 = createEntry('2021-11-05T00:00:02Z', 'inv1');
const entry2 = createEntry('2021-11-05T00:00:01Z', 'inv1');
const entry3 = createEntry('2021-11-05T00:00:00Z', 'inv1');
const entry4 = createEntry('2021-11-04T00:00:00Z', 'inv1');
const entry5 = createEntry('2021-11-03T00:00:00Z', 'inv1');

describe('TestHistoryVariantLoader', () => {
  it('loadUntil should work correctly', async () => {
    // Set up.
    const stub = sinon.stub<[QueryTestHistoryRequest, CacheOption], Promise<QueryTestHistoryResponse>>();
    stub.onCall(0).resolves({ verdicts: [entry1, entry2], nextPageToken: 'page2' });
    stub.onCall(1).resolves({ verdicts: [entry3, entry4], nextPageToken: 'page3' });
    stub.onCall(2).resolves({ verdicts: [entry5] });
    const thvLoader = new TestHistoryVariantLoader(
      'project:realm',
      'test-id',
      variant,
      (resolve) => resolve.toFormat('yyyy-MM-dd'),
      {
        query: stub,
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
    expect(stub.getCall(0).args[0]).containSubset({
      project: 'project',
      testId: 'test-id',
      predicate: {
        subRealm: 'realm',
        variantPredicate: { equals: variant },
      },
    });
    expect(stub.getCall(1).args[0]).containSubset({
      project: 'project',
      testId: 'test-id',
      predicate: {
        subRealm: 'realm',
        variantPredicate: { equals: variant },
      },
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
    expect(stub.getCall(2).args[0]).containSubset({
      project: 'project',
      testId: 'test-id',
      predicate: {
        subRealm: 'realm',
        variantPredicate: { equals: variant },
      },
      pageToken: 'page3',
    });

    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-04T00:00:00Z'), true), [entry4]);
    // We've reached the end of the pages. Treat entries from '2021-11-03' as
    // loaded.
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-11-03T00:00:00Z'), true), [entry5]);
    // Any past date should be considered loaded and return empty array.
    assert.deepEqual(thvLoader.getEntries(DateTime.fromISO('2021-10-01T00:00:00Z'), true), []);
  });
});
