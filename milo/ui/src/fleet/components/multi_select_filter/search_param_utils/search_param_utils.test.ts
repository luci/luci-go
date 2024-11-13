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

import { SelectedFilters } from '../types';

import { parseFilters, stringifyFilters } from './search_param_utils';

const sharedTestCases: [SelectedFilters, string][] = [
  [{}, ''],
  [
    {
      nameSpace: {
        key: ['value'],
      },
    },
    'nameSpace.key = value',
  ],
  [
    {
      nameSpace: {
        key1: ['value1'],
        key2: ['value2'],
      },
    },
    'nameSpace.key1 = value1 nameSpace.key2 = value2',
  ],
  [
    {
      nameSpace1: {
        key1: ['value1'],
        key2: ['value2'],
      },
      nameSpace2: {
        key1: ['value1'],
        key2: ['value2'],
      },
    },
    'nameSpace1.key1 = value1 nameSpace1.key2 = value2 nameSpace2.key1 = value1 nameSpace2.key2 = value2',
  ],
  [
    {
      nameSpace: {
        key1: ['value1', 'value2'],
        key2: ['value1', 'value2'],
      },
    },
    'nameSpace.key1 = (value1 AND value2) nameSpace.key2 = (value1 AND value2)',
  ],
  [
    {
      nameSpace: {
        key1: ['value1', 'value2'],
      },
    },
    'nameSpace.key1 = (value1 AND value2)',
  ],
  [
    {
      nameSpace: {
        key1: ['value1', 'value2'],
      },
      nameSpace2: {
        key: ['value'],
      },
    },
    'nameSpace.key1 = (value1 AND value2) nameSpace2.key = value',
  ],
];

const justStringifyCases: [SelectedFilters, string][] = [
  [
    {
      nameSpace: {
        key: ['value1'],
        emptyKey: [],
      },
    },
    'nameSpace.key = value1',
  ],
  [
    {
      nameSpace: {
        emptyKey: [],
      },
    },
    '',
  ],
  [
    {
      unusedNameSpace: {},
    },
    '',
  ],
];

const justParseCases: [SelectedFilters, string][] = [
  [
    {
      nameSpace: {
        key1: ['value1', 'value2'],
      },
    },
    'nameSpace.key1= ( value1 AND value2)',
  ],
  [
    {
      nameSpace: {
        key: ['value1'],
      },
    },
    'nameSpace.key=value1',
  ],
];

describe('multi_select_search_param_utils', () => {
  describe('stringifyFilters', () => {
    it.each(sharedTestCases.concat(justStringifyCases))(
      'stringifyFilters(%o)',
      (input, expectedOutput) => {
        expect(stringifyFilters(input)).toBe(expectedOutput);
      },
    );
  });

  describe('parseFilters', () => {
    it.each(sharedTestCases.concat(justParseCases).map(([a, b]) => [b, a]))(
      'parseFilters(%s)',
      (input, expectedOutput) => {
        const realOutput = parseFilters(input);

        try {
          expect(new Set(Object.keys(realOutput))).toEqual(
            new Set(Object.keys(expectedOutput)),
          );
          for (const nameSpace of Object.keys(expectedOutput)) {
            for (const key of Object.keys(expectedOutput[nameSpace])) {
              const realValues = realOutput[nameSpace]?.[key];
              const expectedValues = expectedOutput[nameSpace]?.[key];

              expect(realValues).not.toBeUndefined();
              expect(new Set(realValues)).toEqual(new Set(expectedValues));
            }
          }
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
      expect(() => parseFilters('nameSpace.key = ( value')).toThrow(
        'Missing closing parenthesis',
      );
    });
    it('throws on a hanging OR', () => {
      expect(() => parseFilters('nameSpace.key = ( value AND )')).toThrow(
        'Found a hanging ANDs',
      );
    });
    it('thows if the namespace is missing', () => {
      expect(() => parseFilters('key = value')).toThrow(
        'Values are expected to have the form nameSpace.key',
      );
    });
  });
});
