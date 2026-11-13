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

import { PerfChartWidget } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import {
  createRawSamplesRequest,
  pointToSelectionContext,
} from './raw_samples_utils';

describe('raw_samples_utils', () => {
  describe('pointToSelectionContext', () => {
    it('returns an empty context for an undefined point', () => {
      expect(pointToSelectionContext(undefined)).toEqual({});
    });

    it('stringifies dimensions and drops value and row count', () => {
      expect(
        pointToSelectionContext({
          build_id: 123,
          num_aggregated_rows: 4,
          test_name: 'test#method',
          value: 1.5,
        }),
      ).toEqual({ build_id: '123', test_name: 'test#method' });
    });
  });

  describe('createRawSamplesRequest', () => {
    it('builds the request', () => {
      const request = createRawSamplesRequest({
        dashboardName: 'dashboardStates/d1',
        extraColumns: ['build_id'],
        orderBy: 'value asc',
        pageSize: 10,
        selectionContext: { test_name: 't' },
        seriesId: 's1',
        seriesIndex: 0,
        widget: PerfChartWidget.fromPartial({}),
        widgetId: 'w1',
      });
      expect(request.name).toBe('dashboardStates/d1');
      expect(request.widgetId).toBe('w1');
      expect(request.seriesId).toBe('s1');
      expect(request.seriesIndex).toBe(0);
      expect(request.selectionContext).toEqual({ test_name: 't' });
      expect(request.extraColumns).toEqual(['build_id']);
      expect(request.orderBy).toBe('value asc');
      expect(request.pageSize).toBe(10);
      expect(request.dashboardContent?.widgets?.[0]?.id).toBe('w1');
    });

    it('drops a negative series index', () => {
      const request = createRawSamplesRequest({
        dashboardName: 'd',
        extraColumns: [],
        orderBy: '',
        pageSize: 1,
        selectionContext: {},
        seriesIndex: -1,
        widget: PerfChartWidget.fromPartial({}),
        widgetId: 'w1',
      });
      expect(request.seriesIndex).toBeUndefined();
    });
  });
});
