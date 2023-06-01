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

import { expect } from '@jest/globals';

import { Variant } from '../../services/luci_analysis';
import {
  parseVariantFilter,
  parseVariantPredicate,
  suggestTestHistoryFilterQuery,
} from './th_filter_query';

const entry1 = {
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
};

const entry2 = {
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
};

const entry3 = {
  variant: { def: { key1: 'val3' } },
  variantHash: 'key1:val3',
};

const entry4 = {
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
};

const entry5 = {
  variant: { def: { key1: 'val2', key2: 'val1' } },
  variantHash: 'key1:val2|key2:val1',
};

const entry6 = {
  variant: { def: { key1: 'val2', key2: 'val3' } },
  variantHash: 'key1:val2|key2:val3',
};

const entry7 = {
  variant: { def: { key1: 'val2', key2: 'val3=val' } },
  variantHash: 'key1:val2|key2:val3=val',
};

const variants = [entry1, entry2, entry3, entry4, entry5, entry6, entry7];

describe('parseVariantFilter', () => {
  it('should filter out variants with no matching variant key-value pair', () => {
    const filter = parseVariantFilter('v:key1=val1');

    const filtered = variants
      .map(
        (v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string]
      )
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    expect(filtered).toEqual([entry1.variant]);
  });

  it("should support variant value with '=' in it", () => {
    const filter = parseVariantFilter('v:key2=val3%3Dval');

    const filtered = variants
      .map(
        (v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string]
      )
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    expect(filtered).toEqual([entry7.variant]);
  });

  it('should support filter with only variant key', () => {
    const filter = parseVariantFilter('V:key2');

    const filtered = variants
      .map(
        (v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string]
      )
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    expect(filtered).toEqual([entry5.variant, entry6.variant, entry7.variant]);
  });

  it('should work with negation', () => {
    const filter = parseVariantFilter('-v:key1=val1');

    const filtered = variants
      .map(
        (v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string]
      )
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    expect(filtered).toEqual([
      entry2.variant,
      entry3.variant,
      entry4.variant,
      entry5.variant,
      entry6.variant,
      entry7.variant,
    ]);
  });

  it('should work multiple queries', () => {
    const filter = parseVariantFilter(
      '-v:key1=val1 STATUS:Unexpected v:key1=val2'
    );

    const filtered = variants
      .map(
        (v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string]
      )
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    expect(filtered).toEqual([
      entry2.variant,
      entry4.variant,
      entry5.variant,
      entry6.variant,
      entry7.variant,
    ]);
  });

  it('should work with variant hash filter', () => {
    const filter = parseVariantFilter('vhash:key1:val2');

    const filtered = variants
      .map(
        (v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string]
      )
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    expect(filtered).toEqual([entry2.variant, entry4.variant]);
  });

  it('should work with negated variant hash filter', () => {
    const filter = parseVariantFilter('-vhash:key1:val2');

    const filtered = variants
      .map(
        (v) => [v.variant || { def: {} }, v.variantHash] as [Variant, string]
      )
      .filter(([v, hash]) => filter(v, hash))
      .map(([v]) => v);
    expect(filtered).toEqual([
      entry1.variant,
      entry3.variant,
      entry5.variant,
      entry6.variant,
      entry7.variant,
    ]);
  });
});

describe('suggestTestHistoryFilterQuery', () => {
  it('should give user some suggestions when the query is empty', () => {
    const suggestions1 = suggestTestHistoryFilterQuery('');
    expect(suggestions1.length).not.toStrictEqual(0);
  });

  it('should not give suggestions when the sub-query is empty', () => {
    const suggestions1 = suggestTestHistoryFilterQuery('VHash:abcd ');
    expect(suggestions1.length).toStrictEqual(0);
  });

  it('should suggest V query when the query prefix is V:', () => {
    const suggestions1 = suggestTestHistoryFilterQuery('V:test_suite');
    expect(suggestions1.find((s) => s.value === 'V:test_suite')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-V:test_suite')).toBeDefined();

    const suggestions2 = suggestTestHistoryFilterQuery('-V:test_suite');
    // When user explicitly typed negative query, don't suggest positive query.
    expect(
      suggestions2.find((s) => s.value === 'V:test_suite')
    ).toBeUndefined();
    expect(suggestions2.find((s) => s.value === '-V:test_suite')).toBeDefined();
  });
});

describe('parseVariantPredicate', () => {
  it('should work with multiple variant key-value filters', () => {
    const predicate = parseVariantPredicate('v:key1=val1 v:key2=val2');
    expect(predicate).toEqual({
      contains: { def: { key1: 'val1', key2: 'val2' } },
    });
  });

  it('should ignore negative filters', () => {
    const predicate = parseVariantPredicate(
      'v:key1=val1 -v:key2=val2 -vhash:902690735d13f8bd'
    );
    expect(predicate).toEqual({ contains: { def: { key1: 'val1' } } });
  });

  it('should prioritize variant hash filter', () => {
    const predicate = parseVariantPredicate(
      'v:key1=val1 vhash:0123456789AbCdEf'
    );
    expect(predicate).toEqual({ hashEquals: '0123456789abcdef' });
  });

  it('should ignore invalid filters', () => {
    const predicate = parseVariantPredicate(
      'v:key1=val1 vhash:invalidhash other:hash'
    );
    expect(predicate).toEqual({ contains: { def: { key1: 'val1' } } });
  });
});
