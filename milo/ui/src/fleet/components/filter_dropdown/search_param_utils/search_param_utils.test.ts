// Copyright 2023 The LUCI Authors.
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

import { format as prettyFormat } from 'pretty-format'; // ES2015 modules

import { SelectedOptions } from '@/fleet/types';

import { parseFilters, stringifyFilters } from './search_param_utils';

const sharedTestCases: [SelectedOptions, string][] = [
  [{}, ''],
  [
    {
      key: ['value'],
    },
    'key = "value"',
  ],
  [
    {
      key1: ['value1'],
      key2: ['value2'],
    },
    'key1 = "value1" key2 = "value2"',
  ],
  [
    {
      'labels."key1"': ['value1'],
      key2: ['value2'],
    },
    'labels."key1" = "value1" key2 = "value2"',
  ],
  [
    {
      key1: ['value1', 'value2'],
      key2: ['value1', 'value2'],
    },
    'key1 = ("value1" OR "value2") key2 = ("value1" OR "value2")',
  ],
  [
    {
      key1: ['value 1', 'value2'],
      key2: ['value'],
    },
    'key1 = ("value 1" OR "value2") key2 = "value"',
  ],
  [
    {
      key1: ['WORKING'],
    },
    'key1 = "WORKING"',
  ],
  [
    {
      key1: ['WORKING', 'BROKEN'],
    },
    'key1 = ("WORKING" OR "BROKEN")',
  ],
];

const justStringifyCases: [SelectedOptions, string][] = [
  [
    {
      key: ['value1'],
      emptyKey: [],
    },
    'key = "value1"',
  ],
  [
    {
      emptyKey: [],
    },
    '',
  ],
];

const justParseCases: [SelectedOptions, string][] = [
  [
    {
      key1: ['value1', 'value2'],
    },
    'key1= ( "value1" OR "value2")',
  ],
  [
    {
      'labels."key"': ['value1'],
    },
    'labels.key="value1"',
  ],
];

describe('multi_select_search_param_utils', () => {
  describe('stringifyFilters', () => {
    it.each(sharedTestCases.concat(justStringifyCases))(
      'test case %#',
      (input, expectedOutput) => {
        expect(stringifyFilters(input)).toBe(expectedOutput);
      },
    );
  });

  describe('parseFilters', () => {
    it.each(sharedTestCases.concat(justParseCases))(
      'test case %#',
      (expectedOutput, input) => {
        const realOutput = parseFilters(input).filters;

        try {
          expect(realOutput).toEqual(expectedOutput);
        } catch {
          throw Error(
            `Expected:\t ${prettyFormat(expectedOutput)},\ngot:\t ${prettyFormat(realOutput)}`,
          );
        }
      },
    );
  });

  describe('parseFilters wrong examples', () => {
    it('throws on a missing close parenthesis', () => {
      expect(parseFilters('labels.key = ( "value"').error?.message).toEqual(
        'Missing closing parenthesis',
      );
    });
    it('throws on a hanging OR', () => {
      expect(
        parseFilters('labels.key = ( "value" OR )').error?.message,
      ).toEqual('Found a hanging ORs');
    });
    it('throws on an unexpected character', () => {
      expect(parseFilters('labels.key = value').error?.message).toEqual(
        "Unexpected character 'v': should be one of '(\"'",
      );
    });
  });
});
