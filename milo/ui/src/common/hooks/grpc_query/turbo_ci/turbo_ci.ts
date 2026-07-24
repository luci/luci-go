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

import { UseQueryResult, useQuery } from '@tanstack/react-query';

import {
  TokenType,
  useGetAuthToken,
} from '@/common/components/auth_state_provider';
import { WrapperQueryOptions } from '@/common/types/query_wrapper_options';
import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { QueryNodesRequest } from '@/proto/turboci/graph/orchestrator/v1/query_nodes_request.pb';
import { QueryNodesResponse } from '@/proto/turboci/graph/orchestrator/v1/query_nodes_response.pb';
import { ReadWorkPlanRequest } from '@/proto/turboci/graph/orchestrator/v1/read_workplan_request.pb';
import { ReadWorkPlanResponse } from '@/proto/turboci/graph/orchestrator/v1/read_workplan_response.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { TurboCIOrchestratorServiceName } from '@/proto/turboci/graph/orchestrator/v1/turbo_ci_orchestrator_service.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { fetchGrpcWeb, useGrpcWebQuery } from '../grpc_query';

export interface TurboCIEnvironment {
  environment: string;
  urlParam: string;
  host: string;
}

export const TURBO_CI_ENVIRONMENTS: TurboCIEnvironment[] = [
  {
    environment: 'prod',
    urlParam: 'prod',
    host: 'https://turboci.pa.googleapis.com',
  },
  {
    environment: 'qual-qa',
    urlParam: 'qual-qa',
    host: 'https://qual-qa-turboci.sandbox.googleapis.com',
  },
  {
    environment: 'qual-qa-atp',
    urlParam: 'qual-qa-atp',
    host: 'https://qual-qa-atp-turboci.sandbox.googleapis.com',
  },
  {
    environment: 'qual-qa-treehugger',
    urlParam: 'qual-qa-treehugger',
    host: 'https://qual-qa-treehugger-turboci.sandbox.googleapis.com',
  },
  {
    environment: 'qual-staging',
    urlParam: 'qual-staging',
    host: 'https://qual-staging-turboci.sandbox.googleapis.com',
  },
];

export const useQueryNodes = (
  request: QueryNodesRequest,
  host: string,
  queryOptions?: WrapperQueryOptions<QueryNodesResponse>,
): UseQueryResult<QueryNodesResponse> => {
  return useGrpcWebQuery<QueryNodesRequest, QueryNodesResponse>(
    {
      host,
      service: TurboCIOrchestratorServiceName,
      method: 'QueryNodes',
      request,
      requestMsg: QueryNodesRequest,
      responseMsg: QueryNodesResponse,
    },
    queryOptions,
  );
};

export const useReadWorkPlan = (
  request: ReadWorkPlanRequest,
  host: string,
  queryOptions?: WrapperQueryOptions<ReadWorkPlanResponse>,
): UseQueryResult<ReadWorkPlanResponse> => {
  return useGrpcWebQuery<ReadWorkPlanRequest, ReadWorkPlanResponse>(
    {
      host,
      service: TurboCIOrchestratorServiceName,
      method: 'ReadWorkPlan',
      request,
      requestMsg: ReadWorkPlanRequest,
      responseMsg: ReadWorkPlanResponse,
    },
    queryOptions,
  );
};

export const usePaginatedReadWorkPlan = (
  request: ReadWorkPlanRequest,
  host: string,
  queryOptions?: WrapperQueryOptions<ReadWorkPlanResponse>,
): UseQueryResult<ReadWorkPlanResponse> => {
  const getAccessToken = useGetAuthToken(TokenType.Access);

  return useQuery({
    ...queryOptions,
    queryKey: [
      'grpc-web-paginated',
      host,
      TurboCIOrchestratorServiceName,
      'ReadWorkPlan',
      request,
    ],
    queryFn: async (): Promise<ReadWorkPlanResponse> => {
      const accessToken = await getAccessToken();
      let currentToken: string | undefined = undefined;
      const allStages: Stage[] = [];
      const allChecks: Check[] = [];
      const allValueData: { [key: string]: ValueData } = {};
      let workplan = undefined;
      let currentAttemptState = undefined;
      let version = undefined;

      do {
        const response: ReadWorkPlanResponse = await fetchGrpcWeb({
          host,
          service: TurboCIOrchestratorServiceName,
          method: 'ReadWorkPlan',
          request: {
            ...request,
            paginationToken: currentToken,
          },
          requestMsg: ReadWorkPlanRequest,
          responseMsg: ReadWorkPlanResponse,
          accessToken,
        });

        if (response.workplan) {
          workplan = response.workplan;
          if (response.workplan.stages) {
            allStages.push(...response.workplan.stages);
          }
          if (response.workplan.checks) {
            allChecks.push(...response.workplan.checks);
          }
        }
        if (response.valueData) {
          Object.assign(allValueData, response.valueData);
        }
        if (response.currentAttemptState) {
          currentAttemptState = response.currentAttemptState;
        }
        if (response.version) {
          version = response.version;
        }

        currentToken = response.paginationToken;
      } while (currentToken);

      return {
        workplan: workplan
          ? {
              ...workplan,
              stages: allStages,
              checks: allChecks,
            }
          : undefined,
        valueData: allValueData,
        currentAttemptState,
        version,
        paginationToken: '',
      };
    },
  });
};
