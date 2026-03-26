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

import { useState } from 'react';
import { useNavigate } from 'react-router';

import {
  DATA_SPEC_ID,
  GLOBAL_TIME_RANGE_COLUMN,
  GLOBAL_TIME_RANGE_FILTER_ID,
  GLOBAL_TIME_RANGE_OPTION_DEFAULT,
} from '@/crystal_ball/constants';
import {
  useCreateDashboardState,
  useGenerateDashboardState,
} from '@/crystal_ball/hooks';
import { extractIdFromName, formatApiError } from '@/crystal_ball/utils';
import {
  CreateDashboardStateRequest,
  DashboardState,
  GenerateDashboardStateRequest,
  PerfDataSource_SourceType,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Hook to handle the Generate Dashboard workflow.
 */
export function useGenerateDashboardWorkflow() {
  const navigate = useNavigate();
  const generateMutation = useGenerateDashboardState();
  const createMutation = useCreateDashboardState();
  const [errorMsg, setErrorMsg] = useState('');

  const generateDashboard = async (
    data: { prompt: string; metricKeys: string[] },
    onSuccess?: () => void,
  ) => {
    setErrorMsg('');
    try {
      const response = await generateMutation.mutateAsync(
        GenerateDashboardStateRequest.fromPartial({
          prompt: data.prompt,
          metricKeys: data.metricKeys,
        }),
      );

      const dashboardState = response.dashboardState;
      if (!dashboardState) {
        throw new Error('No dashboard state returned from API');
      }

      const baseDashboardContent = dashboardState.dashboardContent || {
        widgets: [],
        dataSpecs: {},
        globalFilters: [],
      };

      const newDataSpecs = { ...baseDashboardContent.dataSpecs };
      if (!newDataSpecs[DATA_SPEC_ID]) {
        newDataSpecs[DATA_SPEC_ID] = {
          displayName: 'Default Data Source',
          source: { type: PerfDataSource_SourceType.TABLE },
        };
      }

      const newGlobalFilters = [...(baseDashboardContent.globalFilters || [])];
      const hasTimeRange = newGlobalFilters.some(
        (f) => f.id === GLOBAL_TIME_RANGE_FILTER_ID,
      );
      if (!hasTimeRange) {
        newGlobalFilters.push(
          PerfFilter.fromPartial({
            id: GLOBAL_TIME_RANGE_FILTER_ID,
            column: GLOBAL_TIME_RANGE_COLUMN,
            displayName: 'Time Range (UTC)',
            range: {
              defaultValue: {
                values: [GLOBAL_TIME_RANGE_OPTION_DEFAULT],
                filterOperator: PerfFilterDefault_FilterOperator.IN_PAST,
              },
            },
          }),
        );
      }

      const newDashboardState = DashboardState.fromPartial({
        ...dashboardState,
        dashboardContent: {
          ...baseDashboardContent,
          dataSpecs: newDataSpecs,
          globalFilters: newGlobalFilters,
        },
      });

      const createResponse = await createMutation.mutateAsync(
        CreateDashboardStateRequest.fromPartial({
          dashboardState: newDashboardState,
        }),
      );

      const parsedResp = createResponse.response;
      const newName = parsedResp?.name;
      if (newName) {
        const newId = extractIdFromName(newName);
        onSuccess?.();
        navigate(`/ui/labs/crystal-ball/dashboards/${newId}`);
      }
    } catch (e) {
      const msg = formatApiError(e, 'Failed to generate dashboard');
      setErrorMsg(msg);
      throw e; // rethrow if caller wants to handle
    }
  };

  return {
    generateDashboard,
    isPending: generateMutation.isPending || createMutation.isPending,
    errorMsg,
    setErrorMsg,
  };
}

/**
 * Hook to handle the Create Dashboard workflow.
 */
export function useCreateDashboardWorkflow() {
  const navigate = useNavigate();
  const createMutation = useCreateDashboardState();
  const [errorMsg, setErrorMsg] = useState('');

  const createDashboard = async (
    data: { displayName: string; description: string },
    onSuccess?: () => void,
  ) => {
    setErrorMsg('');
    try {
      const response = await createMutation.mutateAsync(
        CreateDashboardStateRequest.fromPartial({
          dashboardState: DashboardState.fromPartial({
            displayName: data.displayName,
            description: data.description,
            dashboardContent: {
              widgets: [],
              dataSpecs: {
                [DATA_SPEC_ID]: {
                  displayName: 'Default Data Source',
                  source: { type: PerfDataSource_SourceType.TABLE },
                },
              },
              globalFilters: [
                PerfFilter.fromPartial({
                  id: GLOBAL_TIME_RANGE_FILTER_ID,
                  column: GLOBAL_TIME_RANGE_COLUMN,
                  displayName: 'Time Range (UTC)',
                  range: {
                    defaultValue: {
                      values: [GLOBAL_TIME_RANGE_OPTION_DEFAULT],
                      filterOperator: PerfFilterDefault_FilterOperator.IN_PAST,
                    },
                  },
                }),
              ],
            },
          }),
        }),
      );

      const parsedResp = response.response;
      const newName = parsedResp?.name;
      if (newName) {
        const newId = extractIdFromName(newName);
        onSuccess?.();
        navigate(`/ui/labs/crystal-ball/dashboards/${newId}`);
      }
    } catch (e) {
      const msg = formatApiError(e, 'Failed to create dashboard');
      setErrorMsg(msg);
      throw e;
    }
  };

  return {
    createDashboard,
    isPending: createMutation.isPending,
    errorMsg,
    setErrorMsg,
  };
}
