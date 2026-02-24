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

import { renderHook, act } from '@testing-library/react';

import { URL_SEARCH_QUERY_PARAM } from '@/crystal_ball/constants';
import { useSearchQuerySync } from '@/crystal_ball/hooks';
import {
  SearchMeasurementsFilter,
  SearchMeasurementsRequest,
} from '@/crystal_ball/types';

// Mock the useSyncedSearchParams hook
const mockSetSearchParams = jest.fn();
const mockParams = new URLSearchParams();

jest.mock('@/generic_libs/hooks/synced_search_params', () => ({
  useSyncedSearchParams: () => [mockParams, mockSetSearchParams],
}));

describe('useSearchQuerySync', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Clear out the URL search params manually
    Array.from(mockParams.keys()).forEach((key) => mockParams.delete(key));
  });

  it('initially reads empty state when no URL params', () => {
    const { result } = renderHook(() => useSearchQuerySync());

    expect(result.current.searchRequestFromUrl).toEqual({});
  });

  describe('updateSearchQuery', () => {
    it('sets the search param with all fields', () => {
      const { result } = renderHook(() => useSearchQuerySync());

      const initialRequest: Partial<SearchMeasurementsRequest> = {
        testNameFilter: 'Example Test',
        buildBranch: 'test-branch',
        buildTarget: 'test-target',
        atpTestNameFilter: 'v2/example/test',
        metricKeys: ['metric1', 'metric2'],
        extraColumns: ['board', 'model'],
      };

      act(() => {
        result.current.updateSearchQuery(initialRequest);
      });

      expect(mockSetSearchParams).toHaveBeenCalledTimes(1);
      // We check that setSearchParams was called with a function
      const updateFn = mockSetSearchParams.mock.calls[0][0];
      const newParams = updateFn(new URLSearchParams());
      const query = newParams.get(URL_SEARCH_QUERY_PARAM);

      expect(query).toContain(
        `testNameFilter:"Example Test" ` +
          `atpTestNameFilter:v2/example/test ` +
          `buildBranch:test-branch ` +
          `buildTarget:test-target ` +
          `metricKeys:metric1,metric2 ` +
          `extraColumns:board,model`,
      );
    });

    it('sets the search param ignoring undefined fields', () => {
      const { result } = renderHook(() => useSearchQuerySync());

      const initialRequest: Partial<SearchMeasurementsRequest> = {
        testNameFilter: 'SingleFieldTest',
        buildCreateStartTime: undefined,
      };

      act(() => {
        result.current.updateSearchQuery(initialRequest);
      });

      const updateFn = mockSetSearchParams.mock.calls[0][0];
      const newParams = updateFn(new URLSearchParams());
      const query = newParams.get(URL_SEARCH_QUERY_PARAM);

      expect(query).toBe('testNameFilter:SingleFieldTest');
      expect(query).not.toContain('buildCreateStartTime');
    });

    it('removes the URL param when given an empty request', () => {
      const { result } = renderHook(() => useSearchQuerySync());

      act(() => {
        result.current.updateSearchQuery({});
      });

      const updateFn = mockSetSearchParams.mock.calls[0][0];
      const newParams = updateFn(new URLSearchParams());

      expect(newParams.has(URL_SEARCH_QUERY_PARAM)).toBe(false);
    });
  });

  describe('clearSearchQuery', () => {
    it('removes the URL param', () => {
      mockParams.set(URL_SEARCH_QUERY_PARAM, 'testNameFilter:"Test"');
      const { result } = renderHook(() => useSearchQuerySync());

      act(() => {
        result.current.clearSearchQuery();
      });

      const updateFn = mockSetSearchParams.mock.calls[0][0];
      const newParams = updateFn(mockParams);

      expect(newParams.has(URL_SEARCH_QUERY_PARAM)).toBe(false);
    });
  });

  describe('deserializeSearchRequest', () => {
    const renderDeserializeHook = (queryString: string) => {
      mockParams.set(URL_SEARCH_QUERY_PARAM, queryString);
      return renderHook(() => useSearchQuerySync());
    };

    it('parses a basic string correctly', () => {
      const queryString =
        `${SearchMeasurementsFilter.TEST}:"Example Test" ` +
        `${SearchMeasurementsFilter.BUILD_BRANCH}:"test-branch"`;
      const expectedRequest: Partial<SearchMeasurementsRequest> = {
        testNameFilter: 'Example Test',
        buildBranch: 'test-branch',
      };
      const { result } = renderDeserializeHook(queryString);
      expect(result.current.searchRequestFromUrl).toEqual(expectedRequest);
    });

    it('parses unquoted strings', () => {
      const queryString =
        `${SearchMeasurementsFilter.TEST}:ExampleGroup.SubGroup#Name ` +
        `${SearchMeasurementsFilter.BUILD_BRANCH}:test-branch`;
      const expectedRequest: Partial<SearchMeasurementsRequest> = {
        testNameFilter: 'ExampleGroup.SubGroup#Name',
        buildBranch: 'test-branch',
      };
      const { result } = renderDeserializeHook(queryString);
      expect(result.current.searchRequestFromUrl).toEqual(expectedRequest);
    });

    it('handles multiple metric keys correctly', () => {
      const queryString = `${SearchMeasurementsFilter.METRIC_KEYS}:"metric1","metric2",metric3`;
      const expectedRequest: Partial<SearchMeasurementsRequest> = {
        metricKeys: ['metric1', 'metric2', 'metric3'],
      };
      const { result } = renderDeserializeHook(queryString);
      expect(result.current.searchRequestFromUrl).toEqual(expectedRequest);
    });

    it('ignores malformed keys', () => {
      const queryString = `invalidKey:"Some Value" ${SearchMeasurementsFilter.TEST}:"Valid Test"`;
      const expectedRequest: Partial<SearchMeasurementsRequest> = {
        testNameFilter: 'Valid Test',
      };
      const { result } = renderDeserializeHook(queryString);
      expect(result.current.searchRequestFromUrl).toEqual(expectedRequest);
    });

    it('handles quoted values containing spaces and delimiters', () => {
      const queryString =
        `${SearchMeasurementsFilter.TEST}:"Test with space, and : colon" ` +
        `${SearchMeasurementsFilter.METRIC_KEYS}:"metric, 1","metric: 2"`;
      const expectedRequest: Partial<SearchMeasurementsRequest> = {
        testNameFilter: 'Test with space, and : colon',
        metricKeys: ['metric, 1', 'metric: 2'],
      };
      const { result } = renderDeserializeHook(queryString);
      expect(result.current.searchRequestFromUrl).toEqual(expectedRequest);
    });

    it('handles escapes correctly', () => {
      const queryString = `${SearchMeasurementsFilter.TEST}:"Test with \\"quote\\" and \\\\slash"`;
      const expectedRequest: Partial<SearchMeasurementsRequest> = {
        testNameFilter: 'Test with "quote" and \\slash',
      };
      const { result } = renderDeserializeHook(queryString);
      expect(result.current.searchRequestFromUrl).toEqual(expectedRequest);
    });

    it('parses all supported fields together', () => {
      const queryString = [
        `${SearchMeasurementsFilter.TEST}:"Ex Test"`,
        `${SearchMeasurementsFilter.ATP_TEST}:"v2/ex/test"`,
        `${SearchMeasurementsFilter.BUILD_BRANCH}:"branch"`,
        `${SearchMeasurementsFilter.BUILD_TARGET}:"target"`,
        `${SearchMeasurementsFilter.METRIC_KEYS}:"m1","m2"`,
        `${SearchMeasurementsFilter.EXTRA_COLUMNS}:"c1","c2"`,
      ].join(' ');

      const expectedRequest: Partial<SearchMeasurementsRequest> = {
        testNameFilter: 'Ex Test',
        atpTestNameFilter: 'v2/ex/test',
        buildBranch: 'branch',
        buildTarget: 'target',
        metricKeys: ['m1', 'm2'],
        extraColumns: ['c1', 'c2'],
      };

      const { result } = renderDeserializeHook(queryString);
      expect(result.current.searchRequestFromUrl).toEqual(expectedRequest);
    });
  });
});
