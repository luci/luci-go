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

import { TestResult, TestStatus, TestVariant, TestVariantStatus } from '../services/resultdb';
import { parseSearchQuery, suggestSearchQuery } from './search_query';

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

const variants = [variant1, variant2, variant3, variant4, variant5, variant6, variant7];

describe('parseSearchQuery', () => {
  describe('query with no type', () => {
    it('should match either test ID or test name', () => {
      const filter = parseSearchQuery('sub');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant6, variant7]);
    });

    it('should be case insensitive', () => {
      const filter = parseSearchQuery('SuB');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant6, variant7]);
    });
  });

  describe('ID query', () => {
    it("should filter out variants whose test ID doesn't match the search text", () => {
      const filter = parseSearchQuery('ID:test-suite-a');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant2]);
    });

    it('should be case insensitive', () => {
      const filter = parseSearchQuery('id:test-suite-b');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant3, variant4, variant5, variant6, variant7]);
    });

    it('should work with negation', () => {
      const filter = parseSearchQuery('-id:test-5');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant2, variant3, variant4]);
    });
  });

  describe('RSTATUS query', () => {
    it('should filter out variants with no matching status', () => {
      const filter = parseSearchQuery('rstatus:pass');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant3, variant5]);
    });

    it('supports multiple statuses', () => {
      const filter = parseSearchQuery('rstatus:pass,fail');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant2, variant3, variant5]);
    });

    it('should work with negation', () => {
      const filter = parseSearchQuery('-rstatus:pass');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant2, variant4, variant6, variant7]);
    });
  });

  describe('ExactID query', () => {
    it('should only keep tests with the same ID', () => {
      const filter = parseSearchQuery('ExactID:invocation-a/test-suite-b/test-5');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant6]);
    });

    it('should be case sensitive', () => {
      const filter = parseSearchQuery('ExactID:invocation-a/test-suite-B/test-5');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant5]);
    });

    it('should work with negation', () => {
      const filter = parseSearchQuery('-ExactID:invocation-a/test-suite-b/test-5');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant2, variant3, variant4, variant5, variant7]);
    });
  });

  describe('V query', () => {
    it('should filter out variants with no matching variant key-value pair', () => {
      const filter = parseSearchQuery('v:key1=val1');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1]);
    });

    it("should support variant value with '=' in it", () => {
      const filter = parseSearchQuery('v:key2=val3=val');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant7]);
    });

    it('should support filter with only variant key', () => {
      const filter = parseSearchQuery('v:key2');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant5, variant6, variant7]);
    });

    it('should work with negation', () => {
      const filter = parseSearchQuery('-v:key1=val1');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant2, variant3, variant4, variant5, variant6, variant7]);
    });
  });

  describe('VHash query', () => {
    it('should filter out variants with no matching variant hash', () => {
      const filter = parseSearchQuery('vhash:key1:val1');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1]);
    });

    it('should work with negation', () => {
      const filter = parseSearchQuery('-vhash:key1:val1');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant2, variant3, variant4, variant5, variant6, variant7]);
    });
  });

  describe('Tag query', () => {
    it('should filter out variants with no matching tag key-value pair', () => {
      const filter = parseSearchQuery('tag:tag-key-1=tag-val-1');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1]);
    });

    it("should support tag value with '=' in it", () => {
      const filter = parseSearchQuery('tag:tag-key-1=tag-val-1=1');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1]);
    });

    it('should support filter with only tag key', () => {
      const filter = parseSearchQuery('tag:tag-key-1');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant2]);
    });

    it('should work with negation', () => {
      const filter = parseSearchQuery('-tag:tag-key-1=tag-val-1');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant2, variant3, variant4, variant5, variant6, variant7]);
    });

    it('should support duplicated tag key', () => {
      const filter = parseSearchQuery('-tag:duplicated-tag-key=second-tag-val');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant3, variant4, variant5, variant6, variant7]);
    });
  });

  describe('Name query', () => {
    it("should filter out variants whose test name doesn't match the search text", () => {
      const filter = parseSearchQuery('Name:test-name');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant2]);
    });

    it('should be case insensitive', () => {
      const filter = parseSearchQuery('Name:test-NAME');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant2]);
    });

    it('should work with negation', () => {
      const filter = parseSearchQuery('-Name:test-name-1');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant2, variant3, variant4, variant5, variant6, variant7]);
    });
  });

  describe('Duration query', () => {
    it('should filter out variants with no run that has the specified duration', () => {
      const filter = parseSearchQuery('Duration:5-10');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1]);
    });

    it('should support decimals', () => {
      const filter = parseSearchQuery('Duration:5.5-10.5');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1]);
    });

    it('should support omitting max duration', () => {
      const filter = parseSearchQuery('Duration:5-');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant2]);
    });

    it('should work with negation', () => {
      const filter = parseSearchQuery('-Duration:5-10');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant2, variant3, variant4, variant5, variant6, variant7]);
    });
  });

  describe('ExactName query', () => {
    it('should only keep tests with the same name', () => {
      const filter = parseSearchQuery('ExactName:test');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant3]);
    });

    it('should be case sensitive', () => {
      const filter = parseSearchQuery('ExactName:tesT');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, []);
    });

    it('should work with negation', () => {
      const filter = parseSearchQuery('-ExactName:test');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant2, variant4, variant5, variant6, variant7]);
    });
  });

  describe('multiple queries', () => {
    it('should be able to combine different types of query', () => {
      const filter = parseSearchQuery('rstatus:pass id:test-3');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant3]);
    });

    it('should be able to combine normal and negative queries', () => {
      const filter = parseSearchQuery('rstatus:pass -rstatus:fail');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant5]);
    });
  });
});

describe('suggestSearchQuery', () => {
  it('should give user some suggestions when the query is empty', () => {
    const suggestions1 = suggestSearchQuery('');
    assert.notStrictEqual(suggestions1.length, 0);
  });

  it('should not give suggestions when the sub-query is empty', () => {
    const suggestions1 = suggestSearchQuery('Status:UNEXPECTED ');
    assert.strictEqual(suggestions1.length, 0);
  });

  it('should give user suggestions based on the last sub-query', () => {
    const suggestions1 = suggestSearchQuery('unexpected Pass');
    assert.isDefined(suggestions1.find((s) => s.value === 'RStatus:Pass'));
    assert.isDefined(suggestions1.find((s) => s.value === '-RStatus:Pass'));
    assert.isUndefined(suggestions1.find((s) => s.value === 'Status:UNEXPECTED'));
    assert.isUndefined(suggestions1.find((s) => s.value === '-Status:UNEXPECTED'));
  });

  it('should suggest run status query with matching status', () => {
    const suggestions1 = suggestSearchQuery('Pass');
    assert.isDefined(suggestions1.find((s) => s.value === 'RStatus:Pass'));
    assert.isDefined(suggestions1.find((s) => s.value === '-RStatus:Pass'));

    const suggestions2 = suggestSearchQuery('Fail');
    assert.isDefined(suggestions2.find((s) => s.value === 'RStatus:Fail'));
    assert.isDefined(suggestions2.find((s) => s.value === '-RStatus:Fail'));

    const suggestions3 = suggestSearchQuery('Crash');
    assert.isDefined(suggestions3.find((s) => s.value === 'RStatus:Crash'));
    assert.isDefined(suggestions3.find((s) => s.value === '-RStatus:Crash'));

    const suggestions4 = suggestSearchQuery('Abort');
    assert.isDefined(suggestions4.find((s) => s.value === 'RStatus:Abort'));
    assert.isDefined(suggestions4.find((s) => s.value === '-RStatus:Abort'));

    const suggestions5 = suggestSearchQuery('Skip');
    assert.isDefined(suggestions5.find((s) => s.value === 'RStatus:Skip'));
    assert.isDefined(suggestions5.find((s) => s.value === '-RStatus:Skip'));
  });

  it('should not suggest run status query with a different status', () => {
    const suggestions1 = suggestSearchQuery('Pass');
    assert.isUndefined(suggestions1.find((s) => s.value === 'RStatus:Fail'));
    assert.isUndefined(suggestions1.find((s) => s.value === '-RStatus:Fail'));
    assert.isUndefined(suggestions1.find((s) => s.value === 'RStatus:Crash'));
    assert.isUndefined(suggestions1.find((s) => s.value === '-RStatus:Crash'));
    assert.isUndefined(suggestions1.find((s) => s.value === 'RStatus:Abort'));
    assert.isUndefined(suggestions1.find((s) => s.value === '-RStatus:Abort'));
    assert.isUndefined(suggestions1.find((s) => s.value === 'RStatus:Skip'));
    assert.isUndefined(suggestions1.find((s) => s.value === '-RStatus:Skip'));
  });

  it('should suggest variant status query with matching status', () => {
    const suggestions1 = suggestSearchQuery('unexpected');
    assert.isDefined(suggestions1.find((s) => s.value === 'Status:UNEXPECTED'));
    assert.isDefined(suggestions1.find((s) => s.value === '-Status:UNEXPECTED'));

    const suggestions2 = suggestSearchQuery('flaky');
    assert.isDefined(suggestions2.find((s) => s.value === 'Status:FLAKY'));
    assert.isDefined(suggestions2.find((s) => s.value === '-Status:FLAKY'));

    const suggestions3 = suggestSearchQuery('exonerated');
    assert.isDefined(suggestions3.find((s) => s.value === 'Status:EXONERATED'));
    assert.isDefined(suggestions3.find((s) => s.value === '-Status:EXONERATED'));

    const suggestions4 = suggestSearchQuery('expected');
    assert.isDefined(suggestions4.find((s) => s.value === 'Status:EXPECTED'));
    assert.isDefined(suggestions4.find((s) => s.value === '-Status:EXPECTED'));
  });

  it('should not suggest variant status query with a different status', () => {
    const suggestions1 = suggestSearchQuery('UNEXPECTED');
    assert.isUndefined(suggestions1.find((s) => s.value === 'Status:FLAKY'));
    assert.isUndefined(suggestions1.find((s) => s.value === '-Status:FLAKY'));
    assert.isUndefined(suggestions1.find((s) => s.value === 'Status:EXONERATED'));
    assert.isUndefined(suggestions1.find((s) => s.value === '-Status:EXONERATED'));
    assert.isUndefined(suggestions1.find((s) => s.value === 'Status:EXPECTED'));
    assert.isUndefined(suggestions1.find((s) => s.value === '-Status:EXPECTED'));
  });

  it('suggestion should be case insensitive', () => {
    const suggestions1 = suggestSearchQuery('PASS');
    assert.isDefined(suggestions1.find((s) => s.value === 'RStatus:Pass'));
    assert.isDefined(suggestions1.find((s) => s.value === '-RStatus:Pass'));

    const suggestions2 = suggestSearchQuery('fail');
    assert.isDefined(suggestions2.find((s) => s.value === 'RStatus:Fail'));
    assert.isDefined(suggestions2.find((s) => s.value === '-RStatus:Fail'));

    const suggestions3 = suggestSearchQuery('CrAsH');
    assert.isDefined(suggestions3.find((s) => s.value === 'RStatus:Crash'));
    assert.isDefined(suggestions3.find((s) => s.value === '-RStatus:Crash'));

    const suggestions4 = suggestSearchQuery('Abort');
    assert.isDefined(suggestions4.find((s) => s.value === 'RStatus:Abort'));
    assert.isDefined(suggestions4.find((s) => s.value === '-RStatus:Abort'));

    const suggestions5 = suggestSearchQuery('sKIP');
    assert.isDefined(suggestions5.find((s) => s.value === 'RStatus:Skip'));
    assert.isDefined(suggestions5.find((s) => s.value === '-RStatus:Skip'));
  });

  it('should suggest ID query', () => {
    const suggestions1 = suggestSearchQuery('ranDom');
    assert.isDefined(suggestions1.find((s) => s.value === 'ID:ranDom'));
    assert.isDefined(suggestions1.find((s) => s.value === '-ID:ranDom'));
  });

  it('should suggest ID query when the query prefix is ID:', () => {
    const suggestions1 = suggestSearchQuery('ID:pass');
    assert.isDefined(suggestions1.find((s) => s.value === 'ID:pass'));
    assert.isDefined(suggestions1.find((s) => s.value === '-ID:pass'));

    const suggestions2 = suggestSearchQuery('-ID:pass');
    // When user explicitly typed negative query, don't suggest positive query.
    assert.isUndefined(suggestions2.find((s) => s.value === 'ID:pass'));
    assert.isDefined(suggestions2.find((s) => s.value === '-ID:pass'));
  });

  it('should suggest ID query when the query type is a substring of ID:', () => {
    const suggestions1 = suggestSearchQuery('i');
    assert.isDefined(suggestions1.find((s) => s.value === 'ID:'));
    assert.isDefined(suggestions1.find((s) => s.value === '-ID:'));
  });

  it('should suggest ID query even when there are other matching queries', () => {
    const suggestions1 = suggestSearchQuery('fail');
    assert.isDefined(suggestions1.find((s) => s.value === 'RStatus:Fail'));
    assert.isDefined(suggestions1.find((s) => s.value === '-RStatus:Fail'));
    assert.isDefined(suggestions1.find((s) => s.value === 'ID:fail'));
    assert.isDefined(suggestions1.find((s) => s.value === '-ID:fail'));
  });

  it('should suggest ExactID query when the query prefix is ExactID:', () => {
    const suggestions1 = suggestSearchQuery('ExactID:pass');
    assert.isDefined(suggestions1.find((s) => s.value === 'ExactID:pass'));
    assert.isDefined(suggestions1.find((s) => s.value === '-ExactID:pass'));

    const suggestions2 = suggestSearchQuery('-ExactID:pass');
    // When user explicitly typed negative query, don't suggest positive query.
    assert.isUndefined(suggestions2.find((s) => s.value === 'ExactID:pass'));
    assert.isDefined(suggestions2.find((s) => s.value === '-ExactID:pass'));
  });

  it('should suggest ExactID query when the query type is a substring of ExactID:', () => {
    const suggestions1 = suggestSearchQuery('xact');
    assert.isDefined(suggestions1.find((s) => s.value === 'ExactID:'));
    assert.isDefined(suggestions1.find((s) => s.value === '-ExactID:'));
  });

  it('should suggest V query when the query prefix is V:', () => {
    const suggestions1 = suggestSearchQuery('V:test_suite');
    assert.isDefined(suggestions1.find((s) => s.value === 'V:test_suite'));
    assert.isDefined(suggestions1.find((s) => s.value === '-V:test_suite'));

    const suggestions2 = suggestSearchQuery('-V:test_suite');
    // When user explicitly typed negative query, don't suggest positive query.
    assert.isUndefined(suggestions2.find((s) => s.value === 'V:test_suite'));
    assert.isDefined(suggestions2.find((s) => s.value === '-V:test_suite'));
  });

  it('should suggest Tag query when the query prefix is Tag:', () => {
    const suggestions1 = suggestSearchQuery('Tag:tag_key');
    assert.isDefined(suggestions1.find((s) => s.value === 'Tag:tag_key'));
    assert.isDefined(suggestions1.find((s) => s.value === '-Tag:tag_key'));

    const suggestions2 = suggestSearchQuery('-Tag:tag_key');
    // When user explicitly typed negative query, don't suggest positive query.
    assert.isUndefined(suggestions2.find((s) => s.value === 'Tag:tag_key'));
    assert.isDefined(suggestions2.find((s) => s.value === '-Tag:tag_key'));
  });

  it('should suggest Duration query when the query prefix is Duration:', () => {
    const suggestions1 = suggestSearchQuery('Duration:10');
    assert.isDefined(suggestions1.find((s) => s.value === 'Duration:10'));
    assert.isDefined(suggestions1.find((s) => s.value === '-Duration:10'));

    const suggestions2 = suggestSearchQuery('-Duration:10');
    // When user explicitly typed negative query, don't suggest positive query.
    assert.isUndefined(suggestions2.find((s) => s.value === 'Duration:10'));
    assert.isDefined(suggestions2.find((s) => s.value === '-Duration:10'));
  });

  it('should suggest VHash query when the query prefix is VHash:', () => {
    const suggestions1 = suggestSearchQuery('VHash:pass');
    assert.isDefined(suggestions1.find((s) => s.value === 'VHash:pass'));
    assert.isDefined(suggestions1.find((s) => s.value === '-VHash:pass'));

    const suggestions2 = suggestSearchQuery('-VHash:pass');
    // When user explicitly typed negative query, don't suggest positive query.
    assert.isUndefined(suggestions2.find((s) => s.value === 'VHash:pass'));
    assert.isDefined(suggestions2.find((s) => s.value === '-VHash:pass'));
  });

  it('should suggest VHash query when the query type is a substring of VHash:', () => {
    const suggestions1 = suggestSearchQuery('hash');
    assert.isDefined(suggestions1.find((s) => s.value === 'VHash:'));
    assert.isDefined(suggestions1.find((s) => s.value === '-VHash:'));
  });
});
