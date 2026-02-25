// Copyright 2026 The LUCI Authors.
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

import { renderHook, waitFor } from '@testing-library/react';

import * as gapiQueryHooks from '@/common/hooks/gapi_query/gapi_query';
import { API_BASE_URL } from '@/crystal_ball/constants';
import {
  useListMeasurementFilterColumns,
  useSuggestMeasurementFilterValues,
} from '@/crystal_ball/hooks';
import {
  ListMeasurementFilterColumnsRequest,
  SuggestMeasurementFilterValuesRequest,
  ListMeasurementFilterColumnsResponse,
  SuggestMeasurementFilterValuesResponse,
  ColumnDataType,
  FilterScope,
} from '@/crystal_ball/types';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

// Mock the imported hooks
jest.mock('@/common/hooks/gapi_query/gapi_query', () => ({
  useGapiQuery: jest.fn(),
}));

const mockedUseGapiQuery = gapiQueryHooks.useGapiQuery as jest.Mock;

const BASE_PATH = `${API_BASE_URL}/v1`;

describe('use_measurement_filter_api', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('useListMeasurementFilterColumns', () => {
    const request: ListMeasurementFilterColumnsRequest = {
      parent: 'dashboardStates/test-dash/dataSpecs/spec1',
      pageSize: 10,
    };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({ data: {}, isLoading: false });
      renderHook(() => useListMeasurementFilterColumns(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/${request.parent}/measurementFilterColumns`,
          method: 'GET',
          params: {
            pageSize: request.pageSize,
            pageToken: undefined,
          },
        },
        undefined,
      );
    });

    it('should pass through options', () => {
      mockedUseGapiQuery.mockReturnValue({ data: {}, isLoading: false });
      const options = { enabled: false };
      renderHook(() => useListMeasurementFilterColumns(request, options), {
        wrapper: FakeContextProvider,
      });
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        expect.any(Object),
        options,
      );
    });

    it('should return mocked response', async () => {
      const mockResponse: ListMeasurementFilterColumnsResponse = {
        measurementFilterColumns: [
          {
            column: 'build_target',
            dataType: ColumnDataType.STRING,
            sampleValues: ['target1', 'target2'],
            primary: true,
            isMetricKey: false,
            applicableScopes: [FilterScope.GLOBAL, FilterScope.WIDGET],
          },
        ],
        nextPageToken: 'nextpage',
      };
      mockedUseGapiQuery.mockReturnValue({
        data: mockResponse,
        isLoading: false,
        isSuccess: true,
      });
      const { result } = renderHook(
        () => useListMeasurementFilterColumns(request),
        {
          wrapper: FakeContextProvider,
        },
      );
      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(result.current.data).toEqual(mockResponse);
    });
  });

  describe('useSuggestMeasurementFilterValues', () => {
    const request: SuggestMeasurementFilterValuesRequest = {
      parent: 'dashboardStates/test-dash/dataSpecs/spec1',
      column: 'build_target',
      query: 'test',
      maxResultCount: 25,
    };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({ data: {}, isLoading: false });
      renderHook(() => useSuggestMeasurementFilterValues(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/${request.parent}/measurementFilterColumns:suggestValues`,
          method: 'GET',
          params: {
            column: request.column,
            query: request.query,
            maxResultCount: request.maxResultCount,
          },
        },
        undefined,
      );
    });

    it('should pass through options', () => {
      mockedUseGapiQuery.mockReturnValue({ data: {}, isLoading: false });
      const options = { staleTime: 10000 };
      renderHook(() => useSuggestMeasurementFilterValues(request, options), {
        wrapper: FakeContextProvider,
      });
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        expect.any(Object),
        options,
      );
    });

    it('should return mocked response', async () => {
      const mockResponse: SuggestMeasurementFilterValuesResponse = {
        values: ['test-target1', 'test-target2'],
      };
      mockedUseGapiQuery.mockReturnValue({
        data: mockResponse,
        isLoading: false,
        isSuccess: true,
      });
      const { result } = renderHook(
        () => useSuggestMeasurementFilterValues(request),
        {
          wrapper: FakeContextProvider,
        },
      );
      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(result.current.data).toEqual(mockResponse);
    });
  });
});
