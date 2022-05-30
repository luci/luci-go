// Copyright 2022 The LUCI Authors.
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
import {
  QueryTestHistoryStatsRequest,
  QueryTestHistoryStatsResponse,
  QueryTestHistoryStatsResponseGroup,
  TestHistoryService,
} from '../services/weetbix';
import { TestHistoryStatsLoader } from './test_history_stats_loader';

function createGroup(timestamp: string, variantHash: string): QueryTestHistoryStatsResponseGroup {
  return {
    partitionTime: timestamp,
    variantHash,
    passedAvgDuration: '1s',
    expectedCount: 1,
  };
}

const group1 = createGroup('2021-11-05T00:00:00Z', 'key1:val1');
const group2 = createGroup('2021-11-05T00:00:00Z', 'key1:val2');
const group3 = createGroup('2021-11-04T00:00:00Z', 'key1:val1');
const group4 = createGroup('2021-11-04T00:00:00Z', 'key1:val2');
const group5 = createGroup('2021-11-03T00:00:00Z', 'key1:val1');

describe('TestHistoryStatsLoader', () => {
  it('loadUntil should work correctly', async () => {
    // Set up.
    const stub = sinon.stub<[QueryTestHistoryStatsRequest, CacheOption], Promise<QueryTestHistoryStatsResponse>>();
    stub.onCall(0).resolves({ groups: [group1, group2], nextPageToken: 'page2' });
    stub.onCall(1).resolves({ groups: [group3, group4], nextPageToken: 'page3' });
    stub.onCall(2).resolves({ groups: [group5] });
    const statsLoader = new TestHistoryStatsLoader(
      'project',
      'realm',
      'test-id',
      DateTime.fromISO('2021-11-05T11:00:00Z'),
      {
        queryStats: stub,
      } as Partial<TestHistoryService> as TestHistoryService
    );

    // Before loading.
    assert.strictEqual(statsLoader.getStats('key1:val1', 0, true), null);
    assert.strictEqual(statsLoader.getStats('key1:val2', 0, true), null);
    assert.strictEqual(statsLoader.getStats('key1:val1', 1, true), null);
    assert.strictEqual(statsLoader.getStats('key1:val2', 1, true), null);
    assert.strictEqual(statsLoader.getStats('key1:val1', 2, true), null);
    assert.strictEqual(statsLoader.getStats('key1:val2', 2, true), null);

    // Load all entries created on or after 2021-11-05.
    await statsLoader.loadUntil(0);
    // The loader should get the 2nd page because it's unclear that all the
    // entries from 2021-11-05 had been loaded after getting the first page.
    assert.deepEqual(stub.getCalls().length, 2);
    assert.deepIncludeProperties(stub.getCall(0).args[0], {
      project: 'project',
      testId: 'test-id',
      predicate: {
        subRealm: 'realm',
      },
    });
    assert.deepIncludeProperties(stub.getCall(1).args[0], {
      project: 'project',
      testId: 'test-id',
      predicate: {
        subRealm: 'realm',
      },
      pageToken: 'page2',
    });

    // After loading.
    assert.deepEqual(statsLoader.getStats('key1:val1', 0, true), group1);
    assert.deepEqual(statsLoader.getStats('key1:val2', 0, true), group2);
    // Not all entries from '2021-11-04' has been loaded. Treat them as unloaded.
    assert.strictEqual(statsLoader.getStats('key1:val1', 1, true), null);
    assert.strictEqual(statsLoader.getStats('key1:val2', 1, true), null);
    assert.strictEqual(statsLoader.getStats('key1:val1', 2, true), null);
    assert.strictEqual(statsLoader.getStats('key1:val2', 2, true), null);

    // Load again with the same date index.
    await statsLoader.loadUntil(0);
    // The loader should not load the next date.
    assert.deepEqual(stub.getCalls().length, 2);

    // Load all entries created on or after 2021-11-04.
    await statsLoader.loadUntil(1);
    assert.deepEqual(stub.getCalls().length, 3);
    assert.deepIncludeProperties(stub.getCall(2).args[0], {
      project: 'project',
      testId: 'test-id',
      predicate: {
        subRealm: 'realm',
      },
      pageToken: 'page3',
    });

    assert.deepEqual(statsLoader.getStats('key1:val1', 0, true), group1);
    assert.deepEqual(statsLoader.getStats('key1:val2', 0, true), group2);
    assert.deepEqual(statsLoader.getStats('key1:val1', 1, true), group3);
    assert.deepEqual(statsLoader.getStats('key1:val2', 1, true), group4);
    assert.deepEqual(statsLoader.getStats('key1:val1', 2, true), group5);
    assert.deepEqual(statsLoader.getStats('key1:val2', 2, true), {
      partitionTime: '2021-11-03T00:00:00.000Z',
      variantHash: 'key1:val2',
    });
  });
});
