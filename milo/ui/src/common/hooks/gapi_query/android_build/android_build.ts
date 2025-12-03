// Copyright 2024 The LUCI Authors.
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

import { useGapiQuery } from '../gapi_query';

import {
  GetInvocationParams,
  InvocationResponse,
  ListBuildsRequest,
  ListBuildsResponse,
} from './types';

const API_BASE_PATH = 'https://androidbuildinternal.googleapis.com/';

/**
 * Gets the current state of a single component
 */
export const useGetInvocation = (
  params: GetInvocationParams,
  queryOptions: WrapperQueryOptions<InvocationResponse>,
): UseQueryResult<InvocationResponse> => {
  return useGapiQuery<InvocationResponse>(
    {
      method: 'GET',
      path:
        API_BASE_PATH +
        `android/internal/build/v3/invocations/${params.invocationId}`,
    },
    queryOptions,
  );
};

export const useListBuilds = (
  params: ListBuildsRequest,
  queryOptions: WrapperQueryOptions<ListBuildsResponse>,
): UseQueryResult<ListBuildsResponse> => {
  const path = 'v4/builds';
  return useGapiQuery<ListBuildsResponse>(
    {
      method: 'GET',
      path: `${API_BASE_PATH}${path}`,
      params: {
        branches: params.branches,
        targets: params.targets,
        start_creation_timestamp: params.start_creation_timestamp,
        end_creation_timestamp: params.end_creation_timestamp,
        sorting_type: params.sorting_type,
        page_token: params.page_token,
        page_size: params.page_size,
        fields:
          params.fields ??
          'next_page_token,previous_page_token,builds(build_id,branch,target,creation_timestamp)',
      },
    },
    queryOptions,
  );
};
