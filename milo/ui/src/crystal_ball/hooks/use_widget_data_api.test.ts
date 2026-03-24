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
import { API_V1_BASE_PATH as BASE_PATH } from '@/crystal_ball/constants';
import { useFetchDashboardWidgetData } from '@/crystal_ball/hooks/use_widget_data_api';
import { createMockQueryResult } from '@/crystal_ball/tests';
import {
  FetchDashboardWidgetDataRequest,
  FetchDashboardWidgetDataResponse,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

jest.mock('@/common/hooks/gapi_query/gapi_query', () => ({
  useGapiQuery: jest.fn(),
  useInfiniteGapiQuery: jest.fn(),
}));

const mockedUseGapiQuery = jest.mocked(gapiQueryHooks.useGapiQuery);

describe('use_widget_data_api', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('useFetchDashboardWidgetData', () => {
    const request: FetchDashboardWidgetDataRequest = {
      name: 'dashboardStates/d1',
      widgetId: 'w1',
    };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue(
        createMockQueryResult({ widgetId: 'w1' }),
      );

      renderHook(() => useFetchDashboardWidgetData(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/dashboardStates/d1:fetchDashboardWidgetData`,
          method: 'POST',
          body: {
            widgetId: 'w1',
          },
        },
        /* options= */ undefined,
      );
    });

    it('should pass through options', () => {
      mockedUseGapiQuery.mockReturnValue(
        createMockQueryResult({ widgetId: 'w1' }),
      );
      const options = { staleTime: 10000 };

      renderHook(() => useFetchDashboardWidgetData(request, options), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        expect.any(Object),
        options,
      );
    });

    it('should return the mocked FetchDashboardWidgetDataResponse', async () => {
      const mockResponse: FetchDashboardWidgetDataResponse = {
        widgetId: 'w1',
        multiMetricChartData: {
          xAxisDataKey: 'time',
          yAxisDataKey: 'value',
          lines: [
            {
              seriesId: 's1',
              legendLabel: 'Series 1',
              dataPoints: [],
              metricField: 'metric1',
            },
          ],
        },
      };

      mockedUseGapiQuery.mockReturnValue(createMockQueryResult(mockResponse));

      const { result } = renderHook(
        () => useFetchDashboardWidgetData(request),
        {
          wrapper: FakeContextProvider,
        },
      );

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual(mockResponse);
      expect(result.current.isLoading).toBe(false);
      expect(result.current.error).toBeNull();
    });
  });
});
