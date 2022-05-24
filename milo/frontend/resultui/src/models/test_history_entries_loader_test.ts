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

import { CacheOption } from '../libs/cached_fn';
import { TestVariant, TestVariantStatus } from '../services/resultdb';
import { GetTestVariantRequest, TestHistoryService } from '../services/test_history_service';
import { TestHistoryEntriesLoader } from './test_history_entries_loader';

const entry1 = {
  invocationIds: ['inv1'],
  invocationTimestamp: '2021-11-05T00:00:01Z',
  variant: { def: { key: 'val1' } },
  variantHash: 'hash1',
  status: TestVariantStatus.EXPECTED,
  averageDuration: '1s',
};
const tv1 = {
  testId: 'test-id',
  variantHash: 'hash1',
  status: TestVariantStatus.EXPECTED,
};

const entry2 = {
  invocationIds: ['inv2'],
  invocationTimestamp: '2021-11-05T00:00:01Z',
  variant: { def: { key: 'val2' } },
  variantHash: 'hash2',
  status: TestVariantStatus.EXPECTED,
  averageDuration: '1s',
};
const tv2 = {
  testId: 'test-id',
  variantHash: 'hash2',
  status: TestVariantStatus.EXPECTED,
};

const entry3 = {
  invocationIds: ['inv3'],
  invocationTimestamp: '2021-11-05T00:00:01Z',
  variant: { def: { key: 'val3' } },
  variantHash: 'hash3',
  status: TestVariantStatus.EXPECTED,
  averageDuration: '1s',
};
const tv3 = {
  testId: 'test-id',
  variantHash: 'hash3',
  status: TestVariantStatus.EXPECTED,
};

const entry4 = {
  invocationIds: ['inv4'],
  invocationTimestamp: '2021-11-05T00:00:01Z',
  variant: { def: { key: 'val4' } },
  variantHash: 'hash4',
  status: TestVariantStatus.EXPECTED,
  averageDuration: '1s',
};
const tv4 = {
  testId: 'test-id',
  variantHash: 'hash4',
  status: TestVariantStatus.EXPECTED,
};

const entry5 = {
  invocationIds: ['inv5'],
  invocationTimestamp: '2021-11-05T00:00:01Z',
  variant: { def: { key: 'val5' } },
  variantHash: 'hash5',
  status: TestVariantStatus.EXPECTED,
  averageDuration: '1s',
};
const tv5 = {
  testId: 'test-id',
  variantHash: 'hash5',
  status: TestVariantStatus.EXPECTED,
};

describe('TestHistoryEntriesLoader', () => {
  let entriesLoader: TestHistoryEntriesLoader;
  let stub = sinon.stub<[GetTestVariantRequest, CacheOption], Promise<TestVariant>>();

  beforeEach(() => {
    stub = sinon.stub();
    stub.onCall(0).resolves(tv1);
    stub.onCall(1).resolves(tv2);
    stub.onCall(2).resolves(tv3);
    stub.onCall(3).resolves(tv4);
    stub.onCall(4).resolves(tv5);
    entriesLoader = new TestHistoryEntriesLoader(
      'test-id',
      [entry1, entry2, entry3, entry4, entry5],
      {
        getTestVariant: stub,
      } as Partial<TestHistoryService> as TestHistoryService,
      2
    );
  });

  it('loadFirstPage should work correctly when called in parallel', async () => {
    const loadPromise1 = entriesLoader.loadFirstPage();
    const loadPromise2 = entriesLoader.loadFirstPage();

    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, false);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);
    assert.strictEqual(stub.callCount, 0);

    await loadPromise1;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);
    assert.deepEqual(entriesLoader.testVariants, [tv1, tv2]);
    assert.strictEqual(stub.callCount, 2);
    assert.deepIncludeProperties(stub.getCall(0).args[0], { invocationIds: ['inv1'] });
    assert.deepIncludeProperties(stub.getCall(1).args[0], { invocationIds: ['inv2'] });

    // The 2nd loading call should be a no-op.
    await loadPromise2;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);
    assert.deepEqual(entriesLoader.testVariants, [tv1, tv2]);
    assert.strictEqual(stub.callCount, 2);
  });

  it('loadNextPage should work correctly when called in parallel', async () => {
    const loadPromise1 = entriesLoader.loadNextPage();
    const loadPromise2 = entriesLoader.loadNextPage();

    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, false);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);
    assert.strictEqual(stub.callCount, 0);

    await loadPromise1;
    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);
    assert.deepEqual(entriesLoader.testVariants, [tv1, tv2]);

    // The 2nd loading call should load extra entries.
    await loadPromise2;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);
    assert.deepEqual(entriesLoader.testVariants, [tv1, tv2, tv3, tv4]);
  });

  it('e2e', async () => {
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, false);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);
    assert.strictEqual(stub.callCount, 0);

    // Load the first page.
    let loadPromise = entriesLoader.loadFirstPage();
    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, false);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);

    // First page loaded.
    await loadPromise;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);
    assert.deepEqual(entriesLoader.testVariants, [tv1, tv2]);
    assert.strictEqual(stub.callCount, 2);
    assert.deepIncludeProperties(stub.getCall(0).args[0], { invocationIds: ['inv1'] });
    assert.deepIncludeProperties(stub.getCall(1).args[0], { invocationIds: ['inv2'] });

    // Calling loadFirstPage again shouldn't trigger loading again.
    loadPromise = entriesLoader.loadFirstPage();
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);
    await loadPromise;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);
    assert.deepEqual(entriesLoader.testVariants, [tv1, tv2]);
    assert.strictEqual(stub.callCount, 2);

    // Load the second page.
    loadPromise = entriesLoader.loadNextPage();
    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);

    // Second page loaded.
    await loadPromise;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);
    assert.deepEqual(entriesLoader.testVariants, [tv1, tv2, tv3, tv4]);
    assert.strictEqual(stub.callCount, 4);
    assert.deepIncludeProperties(stub.getCall(2).args[0], { invocationIds: ['inv3'] });
    assert.deepIncludeProperties(stub.getCall(3).args[0], { invocationIds: ['inv4'] });

    // Load the third page.
    loadPromise = entriesLoader.loadNextPage();
    assert.strictEqual(entriesLoader.isLoading, true);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, false);

    // Third page loaded.
    await loadPromise;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, true);
    assert.deepEqual(entriesLoader.testVariants, [tv1, tv2, tv3, tv4, tv5]);
    assert.strictEqual(stub.callCount, 5);
    assert.deepIncludeProperties(stub.getCall(4).args[0], { invocationIds: ['inv5'] });

    // Calling loadNextPage when all variants are again shouldn't trigger
    // loading again.
    loadPromise = entriesLoader.loadNextPage();
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, true);
    await loadPromise;
    assert.strictEqual(entriesLoader.isLoading, false);
    assert.strictEqual(entriesLoader.loadedFirstPage, true);
    assert.strictEqual(entriesLoader.loadedAllTestVariants, true);
    assert.deepEqual(entriesLoader.testVariants, [tv1, tv2, tv3, tv4, tv5]);
    assert.strictEqual(stub.callCount, 5);
  });
});
