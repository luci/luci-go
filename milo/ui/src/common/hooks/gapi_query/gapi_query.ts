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

import {
  UseQueryOptions,
  UseQueryResult,
  useInfiniteQuery,
  useQuery,
} from '@tanstack/react-query';

import {
  TokenType,
  useGetAuthToken,
} from '@/common/components/auth_state_provider';
import {
  WrapperInfiniteQueryOptions,
  WrapperQueryOptions,
} from '@/common/types/query_wrapper_options';

export const useGapiQuery = <Response>(
  args: gapi.client.RequestOptions,
  queryOptions?: WrapperQueryOptions<Response>,
): UseQueryResult<Response> => {
  const getAccessToken = useGetAuthToken(TokenType.Access);
  const options: UseQueryOptions<Response> = {
    ...queryOptions,
    queryKey: ['gapi', args.method, args.path, args.params, args.body],
    queryFn: async (): Promise<Response> => {
      const accessToken = await getAccessToken();
      gapi.client.setToken({ access_token: accessToken });
      const response = await gapi.client.request(args);
      return JSON.parse(response.body);
    },
  };
  return useQuery(options);
};

interface InfiniteGapiQueryResponse {
  nextPageToken: string;
}

export function useInfiniteGapiQuery<
  Response extends InfiniteGapiQueryResponse,
>(
  args: gapi.client.RequestOptions,
  queryOptions?: WrapperInfiniteQueryOptions<Response>,
) {
  const getAccessToken = useGetAuthToken(TokenType.Access);
  return useInfiniteQuery({
    initialPageParam: '',
    ...queryOptions,
    queryKey: ['gapi', args.method, args.path, args.params, args.body],
    queryFn: async ({ pageParam }): Promise<Response> => {
      const accessToken = await getAccessToken();
      gapi.client.setToken({ access_token: accessToken });
      if (pageParam) {
        args.body = {
          ...args.body,
          pageToken: pageParam,
        };
      }
      const response = await gapi.client.request(args);
      return JSON.parse(response.body);
    },
    getNextPageParam: (response) => {
      return response.nextPageToken;
    },
  });
}
