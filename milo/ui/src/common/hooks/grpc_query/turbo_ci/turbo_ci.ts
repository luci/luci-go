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
import { QueryNodesRequest } from '@/proto/turboci/graph/orchestrator/v1/query_nodes_request.pb';
import { QueryNodesResponse } from '@/proto/turboci/graph/orchestrator/v1/query_nodes_response.pb';
import { TurboCIOrchestratorServiceName } from '@/proto/turboci/graph/orchestrator/v1/turbo_ci_orchestrator_service.pb';

import { useGrpcWebQuery } from '../grpc_query';

const HOST = 'https://turboci.pa.googleapis.com';

export const useQueryNodes = (
  request: QueryNodesRequest,
  queryOptions?: WrapperQueryOptions<QueryNodesResponse>,
): UseQueryResult<QueryNodesResponse> => {
  return useGrpcWebQuery<QueryNodesRequest, QueryNodesResponse>(
    {
      host: HOST,
      service: TurboCIOrchestratorServiceName,
      method: 'QueryNodes',
      request,
      requestMsg: QueryNodesRequest,
      responseMsg: QueryNodesResponse,
    },
    queryOptions,
  );
};
