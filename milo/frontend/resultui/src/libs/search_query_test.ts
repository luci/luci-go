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
import { parseSearchQuery } from './search_query';


const variant1: TestVariant = {
  testId: 'invocation-a/test-suite-a/test-1',
  variant: {'def': {'key1': 'val1'}},
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
  results: [
    {
      result: {
        status: TestStatus.Fail,
      } as TestResult,
    },
    {
      result: {
        status: TestStatus.Fail,
      } as TestResult,
    },
    {
      result: {
        status: TestStatus.Skip,
      } as TestResult,
    },
  ],
};

const variant2: TestVariant = {
  testId: 'invocation-a/test-suite-a/test-2',
  variant: {def: {'key1': 'val2'}},
  variantHash: 'key1:val2',
  status: TestVariantStatus.UNEXPECTED,
  results: [
    {
      result: {
        status: TestStatus.Fail,
      } as TestResult,
    },
    {
      result: {
        status: TestStatus.Fail,
      } as TestResult,
    },
  ],
};

const variant3: TestVariant = {
  testId: 'invocation-a/test-suite-b/test-3',
  variant: {def: {'key1': 'val3'}},
  variantHash: 'key1:val3',
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
  variant: {'def': {'key1': 'val2'}},
  variantHash: 'key1:val2',
  status: TestVariantStatus.EXONERATED,
};

const variant5: TestVariant = {
  testId: 'invocation-a/test-suite-B/test-5',
  variant: {def: {'key1': 'val2', 'key2': 'val1'}},
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


const variants = [
  variant1,
  variant2,
  variant3,
  variant4,
  variant5,
];


describe('parseSearchQuery', () => {
  describe('ID query', () => {
    it('should filter out variants whose test ID doesn\'t match the search text', () => {
      const filter = parseSearchQuery('ID:test-suite-a');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant1, variant2]);
    });

    it('should be case insensitive', () => {
      const filter = parseSearchQuery('id:test-suite-b');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant3, variant4, variant5]);
    });

    it('should interpret normal query as ID query', () => {
      const filter = parseSearchQuery('test-5');
      const filtered = variants.filter(filter);
      assert.deepEqual(filtered, [variant5]);
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
      assert.deepEqual(filtered, [variant1, variant2, variant4]);
    });
  });


  describe('multiple query', () => {
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
