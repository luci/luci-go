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

import {
  combineAipFilters,
  escapeAipValue,
  formatAipClause,
  getFilterQueryString,
  getLegacyFilterOrQuery,
  getVisibilityModel,
  quoteAipKey,
} from '@/fleet/utils/search_param';

describe('data_table_search_param_utils', () => {
  describe('escapeAipValue', () => {
    it('should escape quotes and backslashes', () => {
      expect(escapeAipValue('hello "world" \\ test')).toEqual(
        'hello \\"world\\" \\\\ test',
      );
    });
  });

  describe('quoteAipKey', () => {
    it('should quote label subkeys containing hyphens', () => {
      expect(quoteAipKey('labels.label-model')).toEqual('labels."label-model"');
      expect(quoteAipKey('state')).toEqual('state');
      expect(quoteAipKey('sw."device_type"')).toEqual('sw."device_type"');
    });
  });

  describe('formatAipClause', () => {
    it('should format single and multi value clauses', () => {
      expect(formatAipClause('pool', ['default'])).toEqual('pool = "default"');
      expect(
        formatAipClause('labels.label-model', ['lapis', 'sapphire']),
      ).toEqual(
        '(labels."label-model" = "lapis" OR labels."label-model" = "sapphire")',
      );
    });
  });

  describe('combineAipFilters', () => {
    it('should combine clauses with AND', () => {
      expect(combineAipFilters('a = "1"', 'b = "2"')).toEqual(
        'a = "1" AND b = "2"',
      );
      expect(combineAipFilters('', 'b = "2"')).toEqual('b = "2"');
    });
  });

  describe('getLegacyFilterOrQuery', () => {
    it('should support legacy query parameters for backwards compatibility', () => {
      expect(
        getLegacyFilterOrQuery(new URLSearchParams([['q', 'test-device-1']])),
      ).toEqual('id = "test-device-1"');
      expect(
        getLegacyFilterOrQuery(
          new URLSearchParams([['filter', 'state = "ready"']]),
        ),
      ).toEqual('state = "ready"');
      expect(
        getLegacyFilterOrQuery(
          new URLSearchParams([['filters', 'pool = "chrome"']]),
        ),
      ).toEqual('pool = "chrome"');
    });
  });

  describe('getFilterQueryString', () => {
    it('should generate search params with filters', () => {
      const res = getFilterQueryString(
        { pool: ['chrome'], state: ['ready'] },
        undefined,
      );
      expect(res).toEqual(
        '?filters=pool+%3D+%22chrome%22+AND+state+%3D+%22ready%22',
      );
    });
  });

  describe('getVisibilityModel', () => {
    it('should work in the basic case', () => {
      const newColumnVisibilityModel: Record<string, boolean> = {
        column1: true,
        column2: false,
        column3: false,
      };

      expect(
        getVisibilityModel(['column1', 'column2', 'column3'], ['column1']),
      ).toEqual(newColumnVisibilityModel);
    });
    it('should ignore columns set to visible that dont really exists', () => {
      const newColumnVisibilityModel: Record<string, boolean> = {
        column1: true,
        column2: false,
        column3: false,
      };

      expect(
        getVisibilityModel(
          ['column1', 'column2', 'column3'],
          ['column1', 'potato'],
        ),
      ).toEqual(newColumnVisibilityModel);
    });
  });
});
