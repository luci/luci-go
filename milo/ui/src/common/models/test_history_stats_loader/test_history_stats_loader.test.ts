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

import { DateTime } from 'luxon';

import {
  QueryTestHistoryStatsRequest,
  QueryTestHistoryStatsResponse,
  QueryTestHistoryStatsResponseGroup,
  TestHistoryService,
} from '@/common/services/luci_analysis';
import { CacheOption } from '@/generic_libs/tools/cached_fn';

import { TestHistoryStatsLoader } from './test_history_stats_loader';

function createGroup(
  timestamp: string,
  variantHash: string,
): QueryTestHistoryStatsResponseGroup {
  return {
    partitionTime: timestamp,
    variantHash,
    passedAvgDuration: '1s',
    expectedCount: 1,
    verdictCounts: {
      passed: 1,
    },
  };
}

const group1 = createGroup('2021-11-05T00:00:00Z', 'key1:val1');
const group2 = createGroup('2021-11-05T00:00:00Z', 'key1:val2');
const group3 = createGroup('2021-11-04T00:00:00Z', 'key1:val1');
const group4 = createGroup('2021-11-04T00:00:00Z', 'key1:val2');
const group5 = createGroup('2021-11-03T00:00:00Z', 'key1:val1');

describe('TestHistoryStatsLoader', () => {
  test('loadUntil should work correctly', async () => {
    // Set up.
    const stub: jest.MockedFunction<
      (
        req: QueryTestHistoryStatsRequest,
        cacheOpt?: CacheOption,
      ) => Promise<QueryTestHistoryStatsResponse>
    > = jest.fn();
    stub.mockResolvedValueOnce({
      groups: [group1, group2],
      nextPageToken: 'page2',
    });
    stub.mockResolvedValueOnce({
      groups: [group3, group4],
      nextPageToken: 'page3',
    });
    stub.mockResolvedValueOnce({ groups: [group5] });
    const statsLoader = new TestHistoryStatsLoader(
      'project',
      'realm',
      'test-id',
      true,
      DateTime.fromISO('2021-11-05T11:00:00Z'),
      { contains: { def: {} } },
      {
        queryStats: stub,
      } as Partial<TestHistoryService> as TestHistoryService,
    );

    // Before loading.
    expect(statsLoader.getStats('key1:val1', 0, true)).toBeNull();
    expect(statsLoader.getStats('key1:val2', 0, true)).toBeNull();
    expect(statsLoader.getStats('key1:val1', 1, true)).toBeNull();
    expect(statsLoader.getStats('key1:val2', 1, true)).toBeNull();
    expect(statsLoader.getStats('key1:val1', 2, true)).toBeNull();
    expect(statsLoader.getStats('key1:val2', 2, true)).toBeNull();

    // Load all entries created on or after 2021-11-05.
    await statsLoader.loadUntil(0);
    // The loader should get the 2nd page because it's unclear that all the
    // entries from 2021-11-05 had been loaded after getting the first page.
    expect(stub.mock.calls.length).toStrictEqual(2);
    expect(stub.mock.calls[0][0]).toMatchObject({
      project: 'project',
      testId: 'test-id',
      predicate: {
        subRealm: 'realm',
      },
      followTestIdRenaming: true,
    });
    expect(stub.mock.calls[1][0]).toMatchObject({
      project: 'project',
      testId: 'test-id',
      predicate: {
        subRealm: 'realm',
      },
      pageToken: 'page2',
      followTestIdRenaming: true,
    });

    // After loading.
    expect(statsLoader.getStats('key1:val1', 0, true)).toEqual(group1);
    expect(statsLoader.getStats('key1:val2', 0, true)).toEqual(group2);
    // Not all entries from '2021-11-04' has been loaded. Treat them as unloaded.
    expect(statsLoader.getStats('key1:val1', 1, true)).toBeNull();
    expect(statsLoader.getStats('key1:val2', 1, true)).toBeNull();
    expect(statsLoader.getStats('key1:val1', 2, true)).toBeNull();
    expect(statsLoader.getStats('key1:val2', 2, true)).toBeNull();

    // Load again with the same date index.
    await statsLoader.loadUntil(0);
    // The loader should not load the next date.
    expect(stub.mock.calls.length).toStrictEqual(2);

    // Load all entries created on or after 2021-11-04.
    await statsLoader.loadUntil(1);
    expect(stub.mock.calls.length).toStrictEqual(3);
    expect(stub.mock.calls[2][0]).toMatchObject({
      project: 'project',
      testId: 'test-id',
      predicate: {
        subRealm: 'realm',
      },
      pageToken: 'page3',
      followTestIdRenaming: true,
    });

    expect(statsLoader.getStats('key1:val1', 0, true)).toEqual(group1);
    expect(statsLoader.getStats('key1:val2', 0, true)).toEqual(group2);
    expect(statsLoader.getStats('key1:val1', 1, true)).toEqual(group3);
    expect(statsLoader.getStats('key1:val2', 1, true)).toEqual(group4);
    expect(statsLoader.getStats('key1:val1', 2, true)).toEqual(group5);
    expect(statsLoader.getStats('key1:val2', 2, true)).toEqual({
      partitionTime: '2021-11-03T00:00:00.000Z',
      variantHash: 'key1:val2',
      verdictCounts: {},
    });
  });
});
