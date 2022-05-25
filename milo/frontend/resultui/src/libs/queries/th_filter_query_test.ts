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

import { TestVariantStatus, Variant } from '../../services/resultdb';
import { TestVariantHistoryEntry } from '../../services/test_history_service';
import { parseVariantFilter, suggestTestHistoryFilterQuery } from './th_filter_query';

const entry1: TestVariantHistoryEntry = {
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
  invocationIds: [],
  invocationTimestamp: '2021-01-01T00:00:00Z',
};

const entry2: TestVariantHistoryEntry = {
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.UNEXPECTED,
  invocationIds: [],
  invocationTimestamp: '2021-01-01T00:00:00Z',
};

const entry3: TestVariantHistoryEntry = {
  variant: { def: { key1: 'val3' } },
  variantHash: 'key1:val3',
  status: TestVariantStatus.FLAKY,
  invocationIds: [],
  invocationTimestamp: '2021-01-01T00:00:00Z',
};

const entry4: TestVariantHistoryEntry = {
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.EXONERATED,
  invocationIds: [],
  invocationTimestamp: '2021-01-01T00:00:00Z',
};

const entry5: TestVariantHistoryEntry = {
  variant: { def: { key1: 'val2', key2: 'val1' } },
  variantHash: 'key1:val2|key2:val1',
  status: TestVariantStatus.EXPECTED,
  invocationIds: [],
  invocationTimestamp: '2021-01-01T00:00:00Z',
};

const entry6: TestVariantHistoryEntry = {
  variant: { def: { key1: 'val2', key2: 'val3' } },
  variantHash: 'key1:val2|key2:val3',
  status: TestVariantStatus.EXPECTED,
  invocationIds: [],
  invocationTimestamp: '2021-01-01T00:00:00Z',
};

const entry7: TestVariantHistoryEntry = {
  variant: { def: { key1: 'val2', key2: 'val3=val' } },
  variantHash: 'key1:val2|key2:val3=val',
  status: TestVariantStatus.EXPECTED,
  invocationIds: [],
  invocationTimestamp: '2021-01-01T00:00:00Z',
};

const variants = [entry1, entry2, entry3, entry4, entry5, entry6, entry7];

describe('parseVariantFilterFromQuery', () => {
  it('should filter out variants with no matching variant key-value pair', () => {
    const filter = parseVariantFilter('v:key1=val1');

    const filtered = variants
      .map((v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string])
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    assert.deepEqual(filtered, [entry1.variant]);
  });

  it("should support variant value with '=' in it", () => {
    const filter = parseVariantFilter('v:key2=val3%3Dval');

    const filtered = variants
      .map((v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string])
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    assert.deepEqual(filtered, [entry7.variant]);
  });

  it('should support filter with only variant key', () => {
    const filter = parseVariantFilter('V:key2');

    const filtered = variants
      .map((v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string])
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    assert.deepEqual(filtered, [entry5.variant, entry6.variant, entry7.variant]);
  });

  it('should work with negation', () => {
    const filter = parseVariantFilter('-v:key1=val1');

    const filtered = variants
      .map((v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string])
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    assert.deepEqual(filtered, [
      entry2.variant,
      entry3.variant,
      entry4.variant,
      entry5.variant,
      entry6.variant,
      entry7.variant,
    ]);
  });

  it('should work multiple queries', () => {
    const filter = parseVariantFilter('-v:key1=val1 STATUS:Unexpected v:key1=val2');

    const filtered = variants
      .map((v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string])
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    assert.deepEqual(filtered, [entry2.variant, entry4.variant, entry5.variant, entry6.variant, entry7.variant]);
  });

  it('should work with variant hash filter', () => {
    const filter = parseVariantFilter('vhash:key1:val2');

    const filtered = variants
      .map((v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string])
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    assert.deepEqual(filtered, [entry2.variant, entry4.variant]);
  });

  it('should work with negated variant hash filter', () => {
    const filter = parseVariantFilter('-vhash:key1:val2');

    const filtered = variants
      .map((v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string])
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    assert.deepEqual(filtered, [entry1.variant, entry3.variant, entry5.variant, entry6.variant, entry7.variant]);
  });
});

describe('suggestTestHistoryFilterQuery', () => {
  it('should give user some suggestions when the query is empty', () => {
    const suggestions1 = suggestTestHistoryFilterQuery('');
    assert.notStrictEqual(suggestions1.length, 0);
  });

  it('should not give suggestions when the sub-query is empty', () => {
    const suggestions1 = suggestTestHistoryFilterQuery('VHash:abcd ');
    assert.strictEqual(suggestions1.length, 0);
  });

  it('should suggest V query when the query prefix is V:', () => {
    const suggestions1 = suggestTestHistoryFilterQuery('V:test_suite');
    assert.isDefined(suggestions1.find((s) => s.value === 'V:test_suite'));
    assert.isDefined(suggestions1.find((s) => s.value === '-V:test_suite'));

    const suggestions2 = suggestTestHistoryFilterQuery('-V:test_suite');
    // When user explicitly typed negative query, don't suggest positive query.
    assert.isUndefined(suggestions2.find((s) => s.value === 'V:test_suite'));
    assert.isDefined(suggestions2.find((s) => s.value === '-V:test_suite'));
  });
});
