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

import { UseQueryResult } from '@tanstack/react-query';

import * as PrpcClients from '@/fleet/hooks/prpc_clients';
import {
  CountRepairMetricsResponse,
  ListRepairMetricsResponse,
  RepairMetric,
  RepairMetric_Priority,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

const MOCK_REPAIR_METRICS: readonly RepairMetric[] = [
  {
    priority: RepairMetric_Priority.BREACHED,
    labName: 'lab1',
    hostGroup: 'group1',
    runTarget: 'target1',
    minimumRepairs: 1,
    devicesOffline: 1,
    totalDevices: 2,
  },
  {
    priority: RepairMetric_Priority.WATCH,
    labName: 'lab2',
    hostGroup: 'group2',
    runTarget: 'target2',
    minimumRepairs: 2,
    devicesOffline: 3,
    totalDevices: 4,
  },
];

const MOCK_COUNT_REPAIR_METRICS: CountRepairMetricsResponse = {
  totalHosts: 10,
  offlineHosts: 2,
  totalDevices: 20,
  offlineDevices: 5,
  totalRepairGroup: 5,
  breachedRepairGroup: 1,
  watchRepairGroup: 2,
  niceRepairGroup: 2,
};

export function createMockUseFleetConsoleClient(
  listRepairMetrics: Partial<
    UseQueryResult<ListRepairMetricsResponse, Error>
  > = {},
  countRepairMetrics: Partial<
    UseQueryResult<CountRepairMetricsResponse, Error>
  > = {},
) {
  return () => {
    return {
      ListRepairMetrics: {
        query: jest.fn().mockReturnValue({
          queryKey: ['ListRepairMetrics'],
          queryFn: jest.fn().mockResolvedValue({
            repairMetrics: MOCK_REPAIR_METRICS,
            nextPageToken: '',
          }),
          data: {
            repairMetrics: MOCK_REPAIR_METRICS,
            nextPageToken: '',
          },
          isPending: false,
          ...listRepairMetrics,
        }),
      },
      CountRepairMetrics: {
        query: jest.fn().mockReturnValue({
          queryKey: ['CountRepairMetrics'],
          queryFn: jest.fn().mockResolvedValue(MOCK_COUNT_REPAIR_METRICS),
          data: MOCK_COUNT_REPAIR_METRICS,
          isPending: false,
          ...countRepairMetrics,
        }),
      },
      GetRepairMetricsDimensions: {
        query: jest.fn().mockReturnValue({
          queryKey: ['GetRepairMetricsDimensions'],
          queryFn: jest.fn().mockResolvedValue({
            dimensions: {
              lab_name: { values: ['lab1', 'lab2'] },
              host_group: { values: ['group1', 'group2'] },
              run_target: { values: ['target1', 'target2'] },
            },
          }),
          data: {
            dimensions: {
              lab_name: { values: ['lab1', 'lab2'] },
              host_group: { values: ['group1', 'group2'] },
              run_target: { values: ['target1', 'target2'] },
            },
          },
          isPending: false,
        }),
      },
    } as unknown as ReturnType<typeof PrpcClients.useFleetConsoleClient>;
  };
}
