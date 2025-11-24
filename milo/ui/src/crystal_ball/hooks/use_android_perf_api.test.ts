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

import { renderHook, waitFor } from '@testing-library/react';

import * as gapiQueryHooks from '@/common/hooks/gapi_query/gapi_query';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  API_BASE_URL,
  useTestConnection,
  useSearchMeasurements,
  useSearchMeasurementsInfinite,
  SearchMeasurementsRequest,
  SearchMeasurementsResponse,
} from './use_android_perf_api';

// Mock the imported hooks
jest.mock('@/common/hooks/gapi_query/gapi_query', () => ({
  useGapiQuery: jest.fn(),
  useInfiniteGapiQuery: jest.fn(),
}));

const mockedUseGapiQuery = gapiQueryHooks.useGapiQuery as jest.Mock;
const mockedUseInfiniteGapiQuery =
  gapiQueryHooks.useInfiniteGapiQuery as jest.Mock;

describe('use_android_perf_api', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('useTestConnection', () => {
    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({ data: {}, isLoading: false });

      renderHook(() => useTestConnection(), { wrapper: FakeContextProvider });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${API_BASE_URL}/v1/perf:testConnection`,
          method: 'GET',
          params: {},
        },
        /* options= */ undefined,
      );
    });

    it('should pass through options', () => {
      mockedUseGapiQuery.mockReturnValue({ data: {}, isLoading: false });
      const options = { enabled: false };

      renderHook(() => useTestConnection(options), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        expect.any(Object),
        options,
      );
    });
  });

  describe('useSearchMeasurements', () => {
    const request: SearchMeasurementsRequest = {
      testNameFilter: 'test%',
      lastNDays: 5,
    };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({
        data: { rows: [], nextPageToken: '' },
        isLoading: false,
      });

      renderHook(() => useSearchMeasurements(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${API_BASE_URL}/v1/measurements:search`,
          method: 'GET',
          params: request,
        },
        /* options= */ undefined,
      );
    });

    it('should pass through options', () => {
      mockedUseGapiQuery.mockReturnValue({
        data: { rows: [], nextPageToken: '' },
        isLoading: false,
      });
      const options = { staleTime: 10000 };

      renderHook(() => useSearchMeasurements(request, options), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        expect.any(Object),
        options,
      );
    });

    it('should return the mocked SearchMeasurementsResponse', async () => {
      const mockResponse: SearchMeasurementsResponse = {
        rows: [
          {
            test: 'TestA',
            buildCreateTime: '2025-10-30T01:00:00Z',
            buildBranch: 'main',
            buildTarget: 'target1',
            metricKey: 'metric1',
            value: 101.2,
            buildId: '12345',
            antsInvocationId: 'I1',
            extraColumns: { board: 'B1' },
          },
          {
            test: 'TestB',
            metricKey: 'metric2',
            value: 202.4,
          },
        ],
        nextPageToken: 'nextPageToken123',
      };

      mockedUseGapiQuery.mockReturnValue({
        data: mockResponse,
        isLoading: false,
        isSuccess: true,
        error: null,
      });

      const { result } = renderHook(() => useSearchMeasurements(request), {
        wrapper: FakeContextProvider,
      });

      // Wait for the hook to settle
      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual(mockResponse);
      expect(result.current.isLoading).toBe(false);
      expect(result.current.error).toBeNull();
    });
  });

  describe('useSearchMeasurementsInfinite', () => {
    const request: Omit<SearchMeasurementsRequest, 'pageToken'> = {
      buildBranch: 'main',
      pageSize: 20,
    };

    it('should call useInfiniteGapiQuery with correct arguments', () => {
      mockedUseInfiniteGapiQuery.mockReturnValue({
        data: { pages: [], pageParams: [] },
        isLoading: false,
        fetchNextPage: jest.fn(),
      });

      renderHook(() => useSearchMeasurementsInfinite(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseInfiniteGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseInfiniteGapiQuery).toHaveBeenCalledWith(
        {
          path: `${API_BASE_URL}/v1/measurements:search`,
          method: 'GET',
          params: request,
        },
        /* options= */ undefined,
      );
    });

    it('should pass through options', () => {
      mockedUseInfiniteGapiQuery.mockReturnValue({
        data: { pages: [], pageParams: [] },
        isLoading: false,
        fetchNextPage: jest.fn(),
      });
      const options = { gcTime: 50000 };

      renderHook(() => useSearchMeasurementsInfinite(request, options), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseInfiniteGapiQuery).toHaveBeenCalledWith(
        expect.any(Object),
        options,
      );
    });
  });
});
