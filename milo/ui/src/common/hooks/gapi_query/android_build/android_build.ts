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

const API_BASE_PATH = 'https://androidbuildinternal.googleapis.com/';

// Note that many of the types in this file can be auto-generated, but are not.
// This is because this is a private API and we only define what we need for our code.

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

export interface GetInvocationParams {
  /**
   * The id of the invocation to get.
   * E.g. "1234"
   */
  invocationId: string;
}

export interface InvocationResponse {
  /**
   * Unique ID. Assigned at creation time by the API backend.
   */
  resource: InvocationJson;
}

export interface InvocationJson {
  invocationId: string;
}
