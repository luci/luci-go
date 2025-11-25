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
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

// Mock the useSyncedSearchParams hook
jest.mock('@/generic_libs/hooks/synced_search_params');
const mockUseSyncedSearchParams = useSyncedSearchParams as jest.Mock;

describe('useSearchQuerySync', () => {
  let mockSetSearchParams: jest.Mock;
  let currentSearchParams: URLSearchParams;

  beforeEach(() => {
    // Reset the mock for each test
    mockSetSearchParams = jest.fn((updater) => {
      const newParams = new URLSearchParams(currentSearchParams);
      const result = updater(newParams);
      currentSearchParams = result;
    });

    currentSearchParams = new URLSearchParams();
    mockUseSyncedSearchParams.mockImplementation(() => [
      currentSearchParams,
      mockSetSearchParams,
    ]);
  });

  const setUrlSearchParam = (qValue: string) => {
    const params = new URLSearchParams();
    if (qValue !== null && qValue !== undefined) {
      params.set(URL_SEARCH_QUERY_PARAM, qValue);
    }
    currentSearchParams = params;
    mockUseSyncedSearchParams.mockImplementation(() => [
      currentSearchParams,
      mockSetSearchParams,
    ]);
  };

  it('should return an empty object when URL has no query param', () => {
    const { result } = renderHook(() => useSearchQuerySync());
    expect(result.current.searchRequestFromUrl).toEqual({});
  });

  it('should deserialize the query param from URL on init', () => {
    const initialRequest: Partial<SearchMeasurementsRequest> = {
      testNameFilter: 'MyTest',
      lastNDays: 7,
      metricKeys: ['metric1', 'metric2'],
    };
    const query = `${SearchMeasurementsFilter.TEST}:MyTest ${
      SearchMeasurementsFilter.LAST_N_DAYS
    }:7 ${SearchMeasurementsFilter.METRIC_KEYS}:metric1,metric2`;
    setUrlSearchParam(query);

    const { result } = renderHook(() => useSearchQuerySync());
    expect(result.current.searchRequestFromUrl).toEqual(initialRequest);
  });

  it('should handle all data types in deserialization', () => {
    const queryString =
      `${SearchMeasurementsFilter.TEST}:TestA ${
        SearchMeasurementsFilter.ATP_TEST
      }:v2/atp/test ${SearchMeasurementsFilter.BUILD_BRANCH}:main ${
        SearchMeasurementsFilter.BUILD_TARGET
      }:target1 ${SearchMeasurementsFilter.LAST_N_DAYS}:10 ` +
      `${SearchMeasurementsFilter.BUILD_CREATE_START_TIME}:12345 ${
        SearchMeasurementsFilter.BUILD_CREATE_END_TIME
      }:67890 ${SearchMeasurementsFilter.METRIC_KEYS}:m1,m2 ${SearchMeasurementsFilter.EXTRA_COLUMNS}:c1,c2`;
    setUrlSearchParam(queryString);

    const { result } = renderHook(() => useSearchQuerySync());
    expect(result.current.searchRequestFromUrl).toEqual({
      testNameFilter: 'TestA',
      atpTestNameFilter: 'v2/atp/test',
      buildBranch: 'main',
      buildTarget: 'target1',
      lastNDays: 10,
      buildCreateStartTime: { seconds: 12345, nanos: 0 },
      buildCreateEndTime: { seconds: 67890, nanos: 0 },
      metricKeys: ['m1', 'm2'],
      extraColumns: ['c1', 'c2'],
    });
  });

  it('should update the URL when updateSearchQuery is called', () => {
    const { result } = renderHook(() => useSearchQuerySync());
    const request: Partial<SearchMeasurementsRequest> = {
      testNameFilter: 'UpdateTest',
      buildBranch: 'git_main',
    };

    act(() => {
      result.current.updateSearchQuery(request);
    });

    expect(mockSetSearchParams).toHaveBeenCalledTimes(1);
    const updater = mockSetSearchParams.mock.calls[0][0];
    const params = new URLSearchParams();
    updater(params);
    expect(params.get(URL_SEARCH_QUERY_PARAM)).toBe(
      `${SearchMeasurementsFilter.TEST}:UpdateTest ${SearchMeasurementsFilter.BUILD_BRANCH}:git_main`,
    );
  });

  it('should serialize all data types correctly', () => {
    const { result } = renderHook(() => useSearchQuerySync());
    const request: Partial<SearchMeasurementsRequest> = {
      testNameFilter: 'TestA',
      atpTestNameFilter: 'v2/atp/test',
      buildBranch: 'main',
      buildTarget: 'target1',
      lastNDays: 10,
      buildCreateStartTime: { seconds: 12345, nanos: 0 },
      buildCreateEndTime: { seconds: 67890, nanos: 0 },
      metricKeys: ['m1', 'm2'],
      extraColumns: ['c1', 'c2'],
    };

    act(() => {
      result.current.updateSearchQuery(request);
    });

    const expectedQuery =
      `${SearchMeasurementsFilter.TEST}:TestA ${
        SearchMeasurementsFilter.ATP_TEST
      }:v2/atp/test ${SearchMeasurementsFilter.BUILD_BRANCH}:main ${
        SearchMeasurementsFilter.BUILD_TARGET
      }:target1 ${SearchMeasurementsFilter.LAST_N_DAYS}:10 ` +
      `${SearchMeasurementsFilter.BUILD_CREATE_START_TIME}:12345 ${
        SearchMeasurementsFilter.BUILD_CREATE_END_TIME
      }:67890 ${SearchMeasurementsFilter.METRIC_KEYS}:m1,m2 ${SearchMeasurementsFilter.EXTRA_COLUMNS}:c1,c2`;
    expect(currentSearchParams.get(URL_SEARCH_QUERY_PARAM)).toBe(expectedQuery);
  });

  it('should clear the query param when updateSearchQuery is called with an empty object', () => {
    setUrlSearchParam(`${SearchMeasurementsFilter.TEST}:SomeTest`);
    const { result } = renderHook(() => useSearchQuerySync());

    act(() => {
      result.current.updateSearchQuery({});
    });
    expect(currentSearchParams.has(URL_SEARCH_QUERY_PARAM)).toBe(false);
  });

  it('should clear the query param when clearSearchQuery is called', () => {
    setUrlSearchParam(`${SearchMeasurementsFilter.TEST}:AnotherTest`);
    const { result } = renderHook(() => useSearchQuerySync());

    act(() => {
      result.current.clearSearchQuery();
    });
    expect(currentSearchParams.has(URL_SEARCH_QUERY_PARAM)).toBe(false);
  });

  it('should quote values with spaces during serialization', () => {
    const { result } = renderHook(() => useSearchQuerySync());
    const request: Partial<SearchMeasurementsRequest> = {
      testNameFilter: 'Test with spaces',
      metricKeys: ['metric A', 'metricB'],
    };
    act(() => {
      result.current.updateSearchQuery(request);
    });
    expect(currentSearchParams.get(URL_SEARCH_QUERY_PARAM)).toBe(
      `${SearchMeasurementsFilter.TEST}:"Test with spaces" ${SearchMeasurementsFilter.METRIC_KEYS}:"metric A",metricB`,
    );
  });

  it('should deserialize quoted values with spaces', () => {
    const query = `${SearchMeasurementsFilter.TEST}:"Test with spaces" ${SearchMeasurementsFilter.METRIC_KEYS}:"metric A",metricB`;
    setUrlSearchParam(query);
    const { result } = renderHook(() => useSearchQuerySync());
    expect(result.current.searchRequestFromUrl).toEqual({
      testNameFilter: 'Test with spaces',
      metricKeys: ['metric A', 'metricB'],
    });
  });

  it('should serialize multiple metric keys with spaces', () => {
    const { result } = renderHook(() => useSearchQuerySync());
    const request: Partial<SearchMeasurementsRequest> = {
      metricKeys: ['metric A', 'metric B'],
    };
    act(() => {
      result.current.updateSearchQuery(request);
    });
    expect(currentSearchParams.get(URL_SEARCH_QUERY_PARAM)).toBe(
      `${SearchMeasurementsFilter.METRIC_KEYS}:"metric A","metric B"`,
    );
  });

  it('should deserialize multiple metric keys with spaces', () => {
    const query = `${SearchMeasurementsFilter.METRIC_KEYS}:"metric A","metric B"`;
    setUrlSearchParam(query);
    const { result } = renderHook(() => useSearchQuerySync());
    expect(result.current.searchRequestFromUrl).toEqual({
      metricKeys: ['metric A', 'metric B'],
    });
  });

  it('should handle colons and commas in values by quoting', () => {
    const { result } = renderHook(() => useSearchQuerySync());
    const request: Partial<SearchMeasurementsRequest> = {
      testNameFilter: 'test:with:colon',
      metricKeys: ['key,with,comma', 'key2'],
    };
    act(() => {
      result.current.updateSearchQuery(request);
    });
    expect(currentSearchParams.get(URL_SEARCH_QUERY_PARAM)).toBe(
      `${SearchMeasurementsFilter.TEST}:"test:with:colon" ${SearchMeasurementsFilter.METRIC_KEYS}:"key,with,comma",key2`,
    );
  });

  it('should deserialize values with colons and commas', () => {
    const query = `${SearchMeasurementsFilter.TEST}:"test:with:colon" ${SearchMeasurementsFilter.METRIC_KEYS}:"key,with,comma",key2`;
    setUrlSearchParam(query);
    const { result } = renderHook(() => useSearchQuerySync());
    expect(result.current.searchRequestFromUrl).toEqual({
      testNameFilter: 'test:with:colon',
      metricKeys: ['key,with,comma', 'key2'],
    });
  });

  it('should handle quotes within values by escaping', () => {
    const { result } = renderHook(() => useSearchQuerySync());
    const request: Partial<SearchMeasurementsRequest> = {
      testNameFilter: 'Test with "quotes"',
      metricKeys: ['a "b" c', 'd'],
    };
    act(() => {
      result.current.updateSearchQuery(request);
    });
    expect(currentSearchParams.get(URL_SEARCH_QUERY_PARAM)).toBe(
      `${SearchMeasurementsFilter.TEST}:"Test with \\"quotes\\"" ${SearchMeasurementsFilter.METRIC_KEYS}:"a \\"b\\" c",d`,
    );
  });

  it('should deserialize values with escaped quotes', () => {
    const query = `${SearchMeasurementsFilter.TEST}:"Test with \\"quotes\\"" ${SearchMeasurementsFilter.METRIC_KEYS}:"a \\"b\\" c",d`;
    setUrlSearchParam(query);
    const { result } = renderHook(() => useSearchQuerySync());
    expect(result.current.searchRequestFromUrl).toEqual({
      testNameFilter: 'Test with "quotes"',
      metricKeys: ['a "b" c', 'd'],
    });
  });

  it('should handle backslashes within values by escaping', () => {
    const { result } = renderHook(() => useSearchQuerySync());
    const request: Partial<SearchMeasurementsRequest> = {
      testNameFilter: 'Test with \\backslash\\',
    };
    act(() => {
      result.current.updateSearchQuery(request);
    });
    expect(currentSearchParams.get(URL_SEARCH_QUERY_PARAM)).toBe(
      `${SearchMeasurementsFilter.TEST}:"Test with \\\\backslash\\\\"`,
    );
  });

  it('should deserialize values with escaped backslashes', () => {
    const query = `${SearchMeasurementsFilter.TEST}:"Test with \\\\backslash\\\\"`;
    setUrlSearchParam(query);
    const { result } = renderHook(() => useSearchQuerySync());
    expect(result.current.searchRequestFromUrl).toEqual({
      testNameFilter: 'Test with \\backslash\\',
    });
  });

  it('should handle complex mixed array values', () => {
    const { result } = renderHook(() => useSearchQuerySync());
    const request: Partial<SearchMeasurementsRequest> = {
      metricKeys: [
        'normal',
        'with space',
        'with,comma',
        'with:colon',
        'with "quote"',
      ],
    };
    act(() => {
      result.current.updateSearchQuery(request);
    });
    expect(currentSearchParams.get(URL_SEARCH_QUERY_PARAM)).toBe(
      `${SearchMeasurementsFilter.METRIC_KEYS}:normal,"with space","with,comma","with:colon","with \\"quote\\""`,
    );
  });

  it('should deserialize complex mixed array values', () => {
    const query = `${SearchMeasurementsFilter.METRIC_KEYS}:normal,"with space","with,comma","with:colon","with \\"quote\\""`;
    setUrlSearchParam(query);
    const { result } = renderHook(() => useSearchQuerySync());
    expect(result.current.searchRequestFromUrl).toEqual({
      metricKeys: [
        'normal',
        'with space',
        'with,comma',
        'with:colon',
        'with "quote"',
      ],
    });
  });

  it('should handle malformed query parts gracefully with new regex', () => {
    const queryString = `${SearchMeasurementsFilter.TEST}:TestA ${
      SearchMeasurementsFilter.LAST_N_DAYS
    }:abc ${SearchMeasurementsFilter.METRIC_KEYS}:m1,,m2 extra-col:column ${SearchMeasurementsFilter.BUILD_BRANCH}:someBranch:val`;
    setUrlSearchParam(queryString);

    const { result } = renderHook(() => useSearchQuerySync());
    expect(result.current.searchRequestFromUrl).toEqual({
      testNameFilter: 'TestA',
      metricKeys: ['m1', 'm2 extra-col:column'],
      buildBranch: 'someBranch:val',
    });
  });

  it('should handle empty value in array gracefully', () => {
    const query = `${SearchMeasurementsFilter.METRIC_KEYS}:a,,c`;
    setUrlSearchParam(query);
    const { result } = renderHook(() => useSearchQuerySync());
    expect(result.current.searchRequestFromUrl).toEqual({
      metricKeys: ['a', 'c'],
    });
  });
});
