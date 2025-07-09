// Copyright 2025 The LUCI Authors.
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
  multiselectFilterToUrlString,
  parseMultiselectFilter,
} from './rri_url_utils';

describe('multi_select_search_param_utils', () => {
  describe('parseMultiselectFilter', () => {
    it('should parse multiple values in parentheses', () => {
      const result = parseMultiselectFilter('("value 1" OR "value 2")');

      expect(result).toEqual(['value 1', 'value 2']);
    });

    it('should handle escaped quotes in values', () => {
      const result = parseMultiselectFilter(
        '("value 1" OR "value with \\"quotes\\"")',
      );

      expect(result).toEqual(['value 1', 'value with "quotes"']);
    });

    it('should parse a single value in parentheses', () => {
      const result = parseMultiselectFilter('("value 1")');

      expect(result).toEqual(['value 1']);
    });

    it('should parse a single value in quotes', () => {
      const result = parseMultiselectFilter('"value 1"');

      expect(result).toEqual(['value 1']);
    });

    it('should parse a single value without quotes', () => {
      const result = parseMultiselectFilter('value 1');

      expect(result).toEqual(['value 1']);
    });

    it('should throw an error for missing closing parenthesis', () => {
      const action = () => parseMultiselectFilter('("value 1" OR "value 2"');

      expect(action).toThrow();
    });

    it('should throw an error for a hanging OR at the end', () => {
      const action = () =>
        parseMultiselectFilter('("value 1" OR "value 2" OR)');

      expect(action).toThrow();
    });

    it('should throw an error for a hanging OR in the middle', () => {
      const action = () =>
        parseMultiselectFilter('("value 1" OR OR "value 2")');

      expect(action).toThrow();
    });

    it('should throw an error for a hanging OR and missing parenthesis', () => {
      const action = () => parseMultiselectFilter('("value 1" OR "value 2" OR');

      expect(action).toThrow();
    });

    it('should throw an error for missing OR between values', () => {
      const action = () => parseMultiselectFilter('("value 1" "value 2")');

      expect(action).toThrow();
    });

    it('should treat unquoted strings with commas as a single value', () => {
      const result = parseMultiselectFilter('value 1, value 2');

      expect(result).toEqual(['value 1, value 2']);
    });

    it('should treat quoted strings with commas as a single value', () => {
      const result = parseMultiselectFilter('"value 1, value 2"');

      expect(result).toEqual(['value 1, value 2']);
    });

    it('should handle extra whitespace', () => {
      const result = parseMultiselectFilter(' (  "value 1"  OR  "value 2"  ) ');

      expect(result).toEqual(['value 1', 'value 2']);
    });

    it('should throw an error for a value with a single quote', () => {
      const action = () => parseMultiselectFilter('("value 1" OR "value 2)');

      expect(action).toThrow();
    });

    it('should throw an error for quotes inside an unquoted value', () => {
      const action = () => parseMultiselectFilter('("value 1" OR v"alu"e 2)');

      expect(action).toThrow();
    });

    it('should throw an error for invalid separator', () => {
      const action = () => parseMultiselectFilter('("value 1" LORD "value 2")');

      expect(action).toThrow();
    });
  });

  describe('multiselectFilterToUrlString', () => {
    it('should format multiple values correctly', () => {
      const result = multiselectFilterToUrlString(['value 1', 'value 2']);

      expect(result).toEqual('("value 1" OR "value 2")');
    });

    it('should format a single value correctly', () => {
      const result = multiselectFilterToUrlString(['value 1']);

      expect(result).toEqual('"value 1"');
    });

    it('should escape quotes in values', () => {
      const result = multiselectFilterToUrlString(['value with "quotes"']);

      expect(result).toEqual('"value with \\"quotes\\""');
    });

    it('should handle multiple values with one having quotes', () => {
      const result = multiselectFilterToUrlString([
        'value 1',
        'value with "quotes"',
      ]);

      expect(result).toEqual('("value 1" OR "value with \\"quotes\\"")');
    });

    it('should return an empty string for no values', () => {
      const result = multiselectFilterToUrlString([]);

      expect(result).toEqual('');
    });
  });
});
