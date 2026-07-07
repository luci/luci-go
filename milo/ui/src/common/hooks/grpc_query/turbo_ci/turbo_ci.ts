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

import { WrapperQueryOptions } from '@/common/types/query_wrapper_options';
import { ReadWorkPlanRequest } from '@/proto/turboci/graph/orchestrator/v1/read_workplan_request.pb';
import { ReadWorkPlanResponse } from '@/proto/turboci/graph/orchestrator/v1/read_workplan_response.pb';
import { TurboCIOrchestratorServiceName } from '@/proto/turboci/graph/orchestrator/v1/turbo_ci_orchestrator_service.pb';

import { useGrpcWebQuery } from '../grpc_query';

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
    environment: 'staging',
    urlParam: 'staging',
    host: 'https://staging-turboci.sandbox.googleapis.com',
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
];

export const useReadWorkPlan = (
  request: ReadWorkPlanRequest,
  host: string,
  queryOptions?: WrapperQueryOptions<ReadWorkPlanResponse>,
): UseQueryResult<ReadWorkPlanResponse> => {
  return useGrpcWebQuery<ReadWorkPlanRequest, ReadWorkPlanResponse>(
    {
      host: host,
      service: TurboCIOrchestratorServiceName,
      method: 'ReadWorkPlan',
      request,
      requestMsg: ReadWorkPlanRequest,
      responseMsg: ReadWorkPlanResponse,
    },
    queryOptions,
  );
};
