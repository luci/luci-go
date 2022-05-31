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
import { DateTime } from 'luxon';
import sinon from 'sinon';

import { CacheOption } from '../libs/cached_fn';
import {
  QueryTestHistoryRequest,
  QueryTestHistoryResponse,
  TestHistoryService,
  TestVerdict,
  TestVerdictStatus,
} from '../services/weetbix';
import { TestHistoryEntriesLoader } from './test_history_entries_loader';

function makeVerdict(partitionTime: string, invocationId: string, status: TestVerdictStatus): TestVerdict {
  return {
    testId: 'test',
    variantHash: 'key1:val1',
    invocationId,
    partitionTime,
    status,
  };
}

const verdict1 = makeVerdict('2022-01-01T00:00:05Z', 'inv1', TestVerdictStatus.UNEXPECTED);
const verdict2 = makeVerdict('2022-01-01T00:00:04Z', 'inv2', TestVerdictStatus.UNEXPECTEDLY_SKIPPED);
const verdict3 = makeVerdict('2022-01-01T00:00:03Z', 'inv3', TestVerdictStatus.FLAKY);
const verdict4 = makeVerdict('2022-01-01T00:00:02Z', 'inv4', TestVerdictStatus.EXONERATED);
const verdict5 = makeVerdict('2022-01-01T00:00:01Z', 'inv5', TestVerdictStatus.EXPECTED);

describe('TestHistoryEntriesLoader', () => {
  let entriesLoader: TestHistoryEntriesLoader;
  let queryHistoryStub = sinon.stub<[QueryTestHistoryRequest, CacheOption], Promise<QueryTestHistoryResponse>>();

  beforeEach(() => {
    queryHistoryStub = sinon.stub();
    queryHistoryStub.onCall(0).resolves({ verdicts: [verdict1, verdict2], nextPageToken: 'page2' });
    queryHistoryStub.onCall(1).resolves({ verdicts: [verdict3, verdict4], nextPageToken: 'page3' });
    queryHistoryStub.onCall(2).resolves({ verdicts: [verdict5] });

    entriesLoader = new TestHistoryEntriesLoader(
      'project',
      'realm',
      'test',
      DateTime.fromISO('2022-01-01T00:00:00Z'),
      { def: { key1: 'val1' } },
      {
        query: queryHistoryStub,
      } as Partial<TestHistoryService> as TestHistoryService,
      2
    );
  });

  it('loadFirstPage should work correctly when called in parallel', async () => {
    const loadPromise1 = entriesLoader.loadFirstPage();
    const loadPromise2 = entriesLoader.loadFirstPage();

    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, false);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);

    await loadPromise1;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);
    assert.deepEqual(entriesLoader.verdictBundles, [
      { verdict: verdict1, variant: { def: { key1: 'val1' } } },
      { verdict: verdict2, variant: { def: { key1: 'val1' } } },
    ]);

    // The 2nd loading call should be a no-op.
    await loadPromise2;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);
    assert.deepEqual(entriesLoader.verdictBundles, [
      { verdict: verdict1, variant: { def: { key1: 'val1' } } },
      { verdict: verdict2, variant: { def: { key1: 'val1' } } },
    ]);
  });

  it('loadNextPage should work correctly when called in parallel', async () => {
    const loadPromise1 = entriesLoader.loadNextPage();
    const loadPromise2 = entriesLoader.loadNextPage();

    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, false);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);

    await loadPromise1;
    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);
    assert.deepEqual(entriesLoader.verdictBundles, [
      { verdict: verdict1, variant: { def: { key1: 'val1' } } },
      { verdict: verdict2, variant: { def: { key1: 'val1' } } },
    ]);

    // The 2nd loading call should load extra entries.
    await loadPromise2;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);
    assert.deepEqual(entriesLoader.verdictBundles, [
      { verdict: verdict1, variant: { def: { key1: 'val1' } } },
      { verdict: verdict2, variant: { def: { key1: 'val1' } } },
      { verdict: verdict3, variant: { def: { key1: 'val1' } } },
      { verdict: verdict4, variant: { def: { key1: 'val1' } } },
    ]);
  });

  it('e2e', async () => {
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, false);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);

    // Load the first page.
    let loadPromise = entriesLoader.loadFirstPage();
    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, false);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);

    // First page loaded.
    await loadPromise;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);
    assert.deepEqual(entriesLoader.verdictBundles, [
      { verdict: verdict1, variant: { def: { key1: 'val1' } } },
      { verdict: verdict2, variant: { def: { key1: 'val1' } } },
    ]);

    // Calling loadFirstPage again shouldn't trigger loading again.
    loadPromise = entriesLoader.loadFirstPage();
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);
    await loadPromise;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);
    assert.deepEqual(entriesLoader.verdictBundles, [
      { verdict: verdict1, variant: { def: { key1: 'val1' } } },
      { verdict: verdict2, variant: { def: { key1: 'val1' } } },
    ]);

    // Load the second page.
    loadPromise = entriesLoader.loadNextPage();
    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);

    // Second page loaded.
    await loadPromise;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);
    assert.deepEqual(entriesLoader.verdictBundles, [
      { verdict: verdict1, variant: { def: { key1: 'val1' } } },
      { verdict: verdict2, variant: { def: { key1: 'val1' } } },
      { verdict: verdict3, variant: { def: { key1: 'val1' } } },
      { verdict: verdict4, variant: { def: { key1: 'val1' } } },
    ]);

    // Load the third page.
    loadPromise = entriesLoader.loadNextPage();
    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, false);

    // Third page loaded.
    await loadPromise;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, true);
    assert.deepEqual(entriesLoader.verdictBundles, [
      { verdict: verdict1, variant: { def: { key1: 'val1' } } },
      { verdict: verdict2, variant: { def: { key1: 'val1' } } },
      { verdict: verdict3, variant: { def: { key1: 'val1' } } },
      { verdict: verdict4, variant: { def: { key1: 'val1' } } },
      { verdict: verdict5, variant: { def: { key1: 'val1' } } },
    ]);

    // Calling loadNextPage when all variants are again shouldn't trigger
    // loading again.
    loadPromise = entriesLoader.loadNextPage();
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, true);
    await loadPromise;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVerdicts, true);
    assert.deepEqual(entriesLoader.verdictBundles, [
      { verdict: verdict1, variant: { def: { key1: 'val1' } } },
      { verdict: verdict2, variant: { def: { key1: 'val1' } } },
      { verdict: verdict3, variant: { def: { key1: 'val1' } } },
      { verdict: verdict4, variant: { def: { key1: 'val1' } } },
      { verdict: verdict5, variant: { def: { key1: 'val1' } } },
    ]);
  });
});
