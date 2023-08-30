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

import {
  TestResult,
  TestStatus,
  TestVariant,
  TestVariantStatus,
} from '@/common/services/resultdb';

import {
  parseTestResultSearchQuery,
  suggestTestResultSearchQuery,
} from './tr_search_query';

const variant1: TestVariant = {
  testId: 'invocation-a/test-suite-a/test-1',
  variant: { def: { key1: 'val1' } },
  variantHash: 'key1:val1',
  testMetadata: {
    name: 'test-name-1',
  },
  status: TestVariantStatus.UNEXPECTED,
  results: [
    {
      result: {
        status: TestStatus.Fail,
        tags: [{ key: 'tag-key-1', value: 'tag-val-1' }],
        duration: '10s',
      } as TestResult,
    },
    {
      result: {
        status: TestStatus.Fail,
        tags: [{ key: 'tag-key-1', value: 'tag-val-1=1' }],
        duration: '15s',
      } as TestResult,
    },
    {
      result: {
        status: TestStatus.Skip,
        tags: [{ key: 'tag-key-2', value: 'tag-val-2' }],
        duration: '20s',
      } as TestResult,
    },
  ],
};

const variant2: TestVariant = {
  testId: 'invocation-a/test-suite-a/test-2',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  testMetadata: {
    name: 'test-name-2',
  },
  status: TestVariantStatus.UNEXPECTED,
  results: [
    {
      result: {
        tags: [{ key: 'tag-key-1', value: 'unknown-val' }],
        status: TestStatus.Fail,
        duration: '30s',
      } as TestResult,
    },
    {
      result: {
        status: TestStatus.Fail,
        tags: [
          { key: 'duplicated-tag-key', value: 'first-tag-val' },
          { key: 'duplicated-tag-key', value: 'second-tag-val' },
        ],
      } as TestResult,
    },
  ],
};

const variant3: TestVariant = {
  testId: 'invocation-a/test-suite-b/test-3',
  variant: { def: { key1: 'val3' } },
  variantHash: 'key1:val3',
  testMetadata: {
    name: 'test',
  },
  status: TestVariantStatus.FLAKY,
  results: [
    {
      result: {
        status: TestStatus.Pass,
      } as TestResult,
    },
    {
      result: {
        status: TestStatus.Fail,
      } as TestResult,
    },
  ],
};

const variant4: TestVariant = {
  testId: 'invocation-a/test-suite-B/test-4',
  variant: { def: { key1: 'val2' } },
  variantHash: 'key1:val2',
  status: TestVariantStatus.EXONERATED,
};

const variant5: TestVariant = {
  testId: 'invocation-a/test-suite-B/test-5',
  variant: { def: { key1: 'val2', key2: 'val1' } },
  variantHash: 'key1:val2|key2:val1',
  status: TestVariantStatus.EXPECTED,
  results: [
    {
      result: {
        status: TestStatus.Pass,
      } as TestResult,
    },
  ],
};

const variant6: TestVariant = {
  testId: 'invocation-a/test-suite-b/test-5',
  variant: { def: { key1: 'val2', key2: 'val3' } },
  variantHash: 'key1:val2|key2:val3',
  testMetadata: {
    name: 'sub',
  },
  status: TestVariantStatus.EXPECTED,
  results: [
    {
      result: {
        status: TestStatus.Skip,
      } as TestResult,
    },
  ],
};

const variant7: TestVariant = {
  testId: 'invocation-a/test-suite-b/test-5/sub',
  variant: { def: { key1: 'val2', key2: 'val3=val' } },
  variantHash: 'key1:val2|key2:val3=val',
  status: TestVariantStatus.EXPECTED,
  results: [
    {
      result: {
        status: TestStatus.Skip,
      } as TestResult,
    },
  ],
};

const variants = [
  variant1,
  variant2,
  variant3,
  variant4,
  variant5,
  variant6,
  variant7,
];

describe('parseTestResultSearchQuery', () => {
  describe('query with no type', () => {
    test('should match either test ID or test name', () => {
      const filter = parseTestResultSearchQuery('sub');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant6, variant7]);
    });

    test('should be case insensitive', () => {
      const filter = parseTestResultSearchQuery('SuB');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant6, variant7]);
    });
  });

  describe('ID query', () => {
    test("should filter out variants whose test ID doesn't match the search text", () => {
      const filter = parseTestResultSearchQuery('ID:test-suite-a');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1, variant2]);
    });

    test('should be case insensitive', () => {
      const filter = parseTestResultSearchQuery('id:test-suite-b');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
    });

    test('should work with negation', () => {
      const filter = parseTestResultSearchQuery('-id:test-5');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1, variant2, variant3, variant4]);
    });
  });

  describe('RSTATUS query', () => {
    test('should filter out variants with no matching status', () => {
      const filter = parseTestResultSearchQuery('rstatus:pass');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant3, variant5]);
    });

    test('supports multiple statuses', () => {
      const filter = parseTestResultSearchQuery('rstatus:pass,fail');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1, variant2, variant3, variant5]);
    });

    test('should work with negation', () => {
      const filter = parseTestResultSearchQuery('-rstatus:pass');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([
        variant1,
        variant2,
        variant4,
        variant6,
        variant7,
      ]);
    });
  });

  describe('ExactID query', () => {
    test('should only keep tests with the same ID', () => {
      const filter = parseTestResultSearchQuery(
        'ExactID:invocation-a/test-suite-b/test-5',
      );
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant6]);
    });

    test('should be case sensitive', () => {
      const filter = parseTestResultSearchQuery(
        'ExactID:invocation-a/test-suite-B/test-5',
      );
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant5]);
    });

    test('should work with negation', () => {
      const filter = parseTestResultSearchQuery(
        '-ExactID:invocation-a/test-suite-b/test-5',
      );
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([
        variant1,
        variant2,
        variant3,
        variant4,
        variant5,
        variant7,
      ]);
    });
  });

  describe('V query', () => {
    test('should filter out variants with no matching variant key-value pair', () => {
      const filter = parseTestResultSearchQuery('v:key1=val1');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1]);
    });

    test("should support variant value with '=' in it", () => {
      const filter = parseTestResultSearchQuery('v:key2=val3%3Dval');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant7]);
    });

    test('should support filter with only variant key', () => {
      const filter = parseTestResultSearchQuery('v:key2');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant5, variant6, variant7]);
    });

    test('should work with negation', () => {
      const filter = parseTestResultSearchQuery('-v:key1=val1');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
    });
  });

  describe('VHash query', () => {
    test('should filter out variants with no matching variant hash', () => {
      const filter = parseTestResultSearchQuery('vhash:key1:val1');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1]);
    });

    test('should work with negation', () => {
      const filter = parseTestResultSearchQuery('-vhash:key1:val1');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
    });
  });

  describe('Tag query', () => {
    test('should filter out variants with no matching tag key-value pair', () => {
      const filter = parseTestResultSearchQuery('tag:tag-key-1=tag-val-1');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1]);
    });

    test("should support tag value with '=' in it", () => {
      const filter = parseTestResultSearchQuery('tag:tag-key-1=tag-val-1=1');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1]);
    });

    test('should support filter with only tag key', () => {
      const filter = parseTestResultSearchQuery('tag:tag-key-1');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1, variant2]);
    });

    test('should work with negation', () => {
      const filter = parseTestResultSearchQuery('-tag:tag-key-1=tag-val-1');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
    });

    test('should support duplicated tag key', () => {
      const filter = parseTestResultSearchQuery(
        '-tag:duplicated-tag-key=second-tag-val',
      );
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([
        variant1,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
    });
  });

  describe('Name query', () => {
    test("should filter out variants whose test name doesn't match the search text", () => {
      const filter = parseTestResultSearchQuery('Name:test-name');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1, variant2]);
    });

    test('should be case insensitive', () => {
      const filter = parseTestResultSearchQuery('Name:test-NAME');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1, variant2]);
    });

    test('should work with negation', () => {
      const filter = parseTestResultSearchQuery('-Name:test-name-1');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
    });
  });

  describe('Duration query', () => {
    test('should filter out variants with no run that has the specified duration', () => {
      const filter = parseTestResultSearchQuery('Duration:5-10');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1]);
    });

    test('should support decimals', () => {
      const filter = parseTestResultSearchQuery('Duration:5.5-10.5');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1]);
    });

    test('should support omitting max duration', () => {
      const filter = parseTestResultSearchQuery('Duration:5-');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant1, variant2]);
    });

    test('should work with negation', () => {
      const filter = parseTestResultSearchQuery('-Duration:5-10');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([
        variant2,
        variant3,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
    });
  });

  describe('ExactName query', () => {
    test('should only keep tests with the same name', () => {
      const filter = parseTestResultSearchQuery('ExactName:test');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant3]);
    });

    test('should be case sensitive', () => {
      const filter = parseTestResultSearchQuery('ExactName:tesT');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([]);
    });

    test('should work with negation', () => {
      const filter = parseTestResultSearchQuery('-ExactName:test');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([
        variant1,
        variant2,
        variant4,
        variant5,
        variant6,
        variant7,
      ]);
    });
  });

  describe('multiple queries', () => {
    test('should be able to combine different types of query', () => {
      const filter = parseTestResultSearchQuery('rstatus:pass id:test-3');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant3]);
    });

    test('should be able to combine normal and negative queries', () => {
      const filter = parseTestResultSearchQuery('rstatus:pass -rstatus:fail');
      const filtered = variants.filter(filter);
      expect(filtered).toEqual([variant5]);
    });
  });
});

describe('suggestTestResultSearchQuery', () => {
  test('should give user some suggestions when the query is empty', () => {
    const suggestions1 = suggestTestResultSearchQuery('');
    expect(suggestions1.length).not.toStrictEqual(0);
  });

  test('should not give suggestions when the sub-query is empty', () => {
    const suggestions1 = suggestTestResultSearchQuery('Status:UNEXPECTED ');
    expect(suggestions1.length).toStrictEqual(0);
  });

  test('should give user suggestions based on the last sub-query', () => {
    const suggestions1 = suggestTestResultSearchQuery('unexpected Pass');
    expect(suggestions1.find((s) => s.value === 'RStatus:Pass')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-RStatus:Pass')).toBeDefined();
    expect(
      suggestions1.find((s) => s.value === 'Status:UNEXPECTED'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === '-Status:UNEXPECTED'),
    ).toBeUndefined();
  });

  test('should suggest run status query with matching status', () => {
    const suggestions1 = suggestTestResultSearchQuery('Pass');
    expect(suggestions1.find((s) => s.value === 'RStatus:Pass')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-RStatus:Pass')).toBeDefined();

    const suggestions2 = suggestTestResultSearchQuery('Fail');
    expect(suggestions2.find((s) => s.value === 'RStatus:Fail')).toBeDefined();
    expect(suggestions2.find((s) => s.value === '-RStatus:Fail')).toBeDefined();

    const suggestions3 = suggestTestResultSearchQuery('Crash');
    expect(suggestions3.find((s) => s.value === 'RStatus:Crash')).toBeDefined();
    expect(
      suggestions3.find((s) => s.value === '-RStatus:Crash'),
    ).toBeDefined();

    const suggestions4 = suggestTestResultSearchQuery('Abort');
    expect(suggestions4.find((s) => s.value === 'RStatus:Abort')).toBeDefined();
    expect(
      suggestions4.find((s) => s.value === '-RStatus:Abort'),
    ).toBeDefined();

    const suggestions5 = suggestTestResultSearchQuery('Skip');
    expect(suggestions5.find((s) => s.value === 'RStatus:Skip')).toBeDefined();
    expect(suggestions5.find((s) => s.value === '-RStatus:Skip')).toBeDefined();
  });

  test('should not suggest run status query with a different status', () => {
    const suggestions1 = suggestTestResultSearchQuery('Pass');
    expect(
      suggestions1.find((s) => s.value === 'RStatus:Fail'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === '-RStatus:Fail'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === 'RStatus:Crash'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === '-RStatus:Crash'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === 'RStatus:Abort'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === '-RStatus:Abort'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === 'RStatus:Skip'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === '-RStatus:Skip'),
    ).toBeUndefined();
  });

  test('should suggest variant status query with matching status', () => {
    const suggestions1 = suggestTestResultSearchQuery('unexpected');
    expect(
      suggestions1.find((s) => s.value === 'Status:UNEXPECTED'),
    ).toBeDefined();
    expect(
      suggestions1.find((s) => s.value === '-Status:UNEXPECTED'),
    ).toBeDefined();

    const suggestions2 = suggestTestResultSearchQuery('flaky');
    expect(suggestions2.find((s) => s.value === 'Status:FLAKY')).toBeDefined();
    expect(suggestions2.find((s) => s.value === '-Status:FLAKY')).toBeDefined();

    const suggestions3 = suggestTestResultSearchQuery('exonerated');
    expect(
      suggestions3.find((s) => s.value === 'Status:EXONERATED'),
    ).toBeDefined();
    expect(
      suggestions3.find((s) => s.value === '-Status:EXONERATED'),
    ).toBeDefined();

    const suggestions4 = suggestTestResultSearchQuery('expected');
    expect(
      suggestions4.find((s) => s.value === 'Status:EXPECTED'),
    ).toBeDefined();
    expect(
      suggestions4.find((s) => s.value === '-Status:EXPECTED'),
    ).toBeDefined();
  });

  test('should not suggest variant status query with a different status', () => {
    const suggestions1 = suggestTestResultSearchQuery('UNEXPECTED');
    expect(
      suggestions1.find((s) => s.value === 'Status:FLAKY'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === '-Status:FLAKY'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === 'Status:EXONERATED'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === '-Status:EXONERATED'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === 'Status:EXPECTED'),
    ).toBeUndefined();
    expect(
      suggestions1.find((s) => s.value === '-Status:EXPECTED'),
    ).toBeUndefined();
  });

  test('suggestion should be case insensitive', () => {
    const suggestions1 = suggestTestResultSearchQuery('PASS');
    expect(suggestions1.find((s) => s.value === 'RStatus:Pass')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-RStatus:Pass')).toBeDefined();

    const suggestions2 = suggestTestResultSearchQuery('fail');
    expect(suggestions2.find((s) => s.value === 'RStatus:Fail')).toBeDefined();
    expect(suggestions2.find((s) => s.value === '-RStatus:Fail')).toBeDefined();

    const suggestions3 = suggestTestResultSearchQuery('CrAsH');
    expect(suggestions3.find((s) => s.value === 'RStatus:Crash')).toBeDefined();
    expect(
      suggestions3.find((s) => s.value === '-RStatus:Crash'),
    ).toBeDefined();

    const suggestions4 = suggestTestResultSearchQuery('Abort');
    expect(suggestions4.find((s) => s.value === 'RStatus:Abort')).toBeDefined();
    expect(
      suggestions4.find((s) => s.value === '-RStatus:Abort'),
    ).toBeDefined();

    const suggestions5 = suggestTestResultSearchQuery('sKIP');
    expect(suggestions5.find((s) => s.value === 'RStatus:Skip')).toBeDefined();
    expect(suggestions5.find((s) => s.value === '-RStatus:Skip')).toBeDefined();
  });

  test('should suggest ID query', () => {
    const suggestions1 = suggestTestResultSearchQuery('ranDom');
    expect(suggestions1.find((s) => s.value === 'ID:ranDom')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-ID:ranDom')).toBeDefined();
  });

  test('should suggest ID query when the query prefix is ID:', () => {
    const suggestions1 = suggestTestResultSearchQuery('ID:pass');
    expect(suggestions1.find((s) => s.value === 'ID:pass')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-ID:pass')).toBeDefined();

    const suggestions2 = suggestTestResultSearchQuery('-ID:pass');
    // When user explicitly typed negative query, don't suggest positive query.
    expect(suggestions2.find((s) => s.value === 'ID:pass')).toBeUndefined();
    expect(suggestions2.find((s) => s.value === '-ID:pass')).toBeDefined();
  });

  test('should suggest ID query when the query type is a substring of ID:', () => {
    const suggestions1 = suggestTestResultSearchQuery('i');
    expect(suggestions1.find((s) => s.value === 'ID:')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-ID:')).toBeDefined();
  });

  test('should suggest ID query even when there are other matching queries', () => {
    const suggestions1 = suggestTestResultSearchQuery('fail');
    expect(suggestions1.find((s) => s.value === 'RStatus:Fail')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-RStatus:Fail')).toBeDefined();
    expect(suggestions1.find((s) => s.value === 'ID:fail')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-ID:fail')).toBeDefined();
  });

  test('should suggest ExactID query when the query prefix is ExactID:', () => {
    const suggestions1 = suggestTestResultSearchQuery('ExactID:pass');
    expect(suggestions1.find((s) => s.value === 'ExactID:pass')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-ExactID:pass')).toBeDefined();

    const suggestions2 = suggestTestResultSearchQuery('-ExactID:pass');
    // When user explicitly typed negative query, don't suggest positive query.
    expect(
      suggestions2.find((s) => s.value === 'ExactID:pass'),
    ).toBeUndefined();
    expect(suggestions2.find((s) => s.value === '-ExactID:pass')).toBeDefined();
  });

  test('should suggest ExactID query when the query type is a substring of ExactID:', () => {
    const suggestions1 = suggestTestResultSearchQuery('xact');
    expect(suggestions1.find((s) => s.value === 'ExactID:')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-ExactID:')).toBeDefined();
  });

  test('should suggest V query when the query prefix is V:', () => {
    const suggestions1 = suggestTestResultSearchQuery('V:test_suite');
    expect(suggestions1.find((s) => s.value === 'V:test_suite')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-V:test_suite')).toBeDefined();

    const suggestions2 = suggestTestResultSearchQuery('-V:test_suite');
    // When user explicitly typed negative query, don't suggest positive query.
    expect(
      suggestions2.find((s) => s.value === 'V:test_suite'),
    ).toBeUndefined();
    expect(suggestions2.find((s) => s.value === '-V:test_suite')).toBeDefined();
  });

  test('should suggest Tag query when the query prefix is Tag:', () => {
    const suggestions1 = suggestTestResultSearchQuery('Tag:tag_key');
    expect(suggestions1.find((s) => s.value === 'Tag:tag_key')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-Tag:tag_key')).toBeDefined();

    const suggestions2 = suggestTestResultSearchQuery('-Tag:tag_key');
    // When user explicitly typed negative query, don't suggest positive query.
    expect(suggestions2.find((s) => s.value === 'Tag:tag_key')).toBeUndefined();
    expect(suggestions2.find((s) => s.value === '-Tag:tag_key')).toBeDefined();
  });

  test('should suggest Duration query when the query prefix is Duration:', () => {
    const suggestions1 = suggestTestResultSearchQuery('Duration:10');
    expect(suggestions1.find((s) => s.value === 'Duration:10')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-Duration:10')).toBeDefined();

    const suggestions2 = suggestTestResultSearchQuery('-Duration:10');
    // When user explicitly typed negative query, don't suggest positive query.
    expect(suggestions2.find((s) => s.value === 'Duration:10')).toBeUndefined();
    expect(suggestions2.find((s) => s.value === '-Duration:10')).toBeDefined();
  });

  test('should suggest VHash query when the query prefix is VHash:', () => {
    const suggestions1 = suggestTestResultSearchQuery('VHash:pass');
    expect(suggestions1.find((s) => s.value === 'VHash:pass')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-VHash:pass')).toBeDefined();

    const suggestions2 = suggestTestResultSearchQuery('-VHash:pass');
    // When user explicitly typed negative query, don't suggest positive query.
    expect(suggestions2.find((s) => s.value === 'VHash:pass')).toBeUndefined();
    expect(suggestions2.find((s) => s.value === '-VHash:pass')).toBeDefined();
  });

  test('should suggest VHash query when the query type is a substring of VHash:', () => {
    const suggestions1 = suggestTestResultSearchQuery('hash');
    expect(suggestions1.find((s) => s.value === 'VHash:')).toBeDefined();
    expect(suggestions1.find((s) => s.value === '-VHash:')).toBeDefined();
  });
});
