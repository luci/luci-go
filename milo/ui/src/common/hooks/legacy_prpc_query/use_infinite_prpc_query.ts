// Copyright 2023 The LUCI Authors.
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
  UseInfiniteQueryOptions,
  UseInfiniteQueryResult,
  useInfiniteQuery,
} from '@tanstack/react-query';

import {
  useAuthState,
  useGetAccessToken,
} from '@/common/components/auth_state_provider';
import { PrpcClientExt } from '@/generic_libs/tools/prpc_client_ext';

import { PrpcMethod, PrpcQueryBaseOptions, genPrpcQueryKey } from './common';

export type PaginatedPrpcMethod<
  Req extends { pageToken?: string },
  Ret extends { nextPageToken?: string },
> = PrpcMethod<Req, Ret>;

export type PrpcServicePaginatedMethodKeys<S> = keyof {
  [MK in keyof S as S[MK] extends PaginatedPrpcMethod<
    // The request type has to be `any` because the argument type must be
    // contra-variant when sub-typing a function.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    any,
    { nextPageToken?: string }
  >
    ? MK
    : never]: S[MK];
};

export type PaginatedPrpcMethodRequest<T> = T extends PaginatedPrpcMethod<
  infer Req,
  infer _Res
>
  ? Req
  : never;

export type PaginatedPrpcMethodResponse<T> = T extends PaginatedPrpcMethod<
  infer _Req,
  infer Res
>
  ? Res
  : never;

export interface UseInfinitePrpcQueryOptions<S, MK, Req, Res, TError, TData>
  extends PrpcQueryBaseOptions<S, MK, Req> {
  /**
   * options will be passed to `useInfiniteQuery` from `@tanstack/react-query`.
   */
  readonly options?: Omit<
    UseInfiniteQueryOptions<
      Res,
      TError,
      TData,
      Res,
      readonly [string, string, string, MK, Req]
    >,
    'queryKey' | 'queryFn' | 'getNextPageParam'
  >;
}

/**
 * @deprecated use `useInfinitePrpcQuery` from `@common/hooks/prpc_query`
 * instead.
 *
 * Call a pRPC method via `@tanstack/react-query`.
 *
 * This hook
 *  * reduces boilerplate, and
 *  * ensures the `queryKey` is populated correctly.
 */
export function useInfinitePrpcQuery<
  S extends object,
  MK extends PrpcServicePaginatedMethodKeys<S>,
  TError = unknown,
  TData = PaginatedPrpcMethodResponse<S[MK]>,
>(
  opts: UseInfinitePrpcQueryOptions<
    S,
    MK,
    PaginatedPrpcMethodRequest<S[MK]>,
    PaginatedPrpcMethodResponse<S[MK]>,
    TError,
    TData
  >,
): UseInfiniteQueryResult<TData, TError> {
  const { host, insecure, Service, method, request, options } = opts;

  const { identity } = useAuthState();
  const getAccessToken = useGetAccessToken();
  const queryKey = genPrpcQueryKey(identity, opts);
  return useInfiniteQuery({
    queryKey,
    queryFn: async ({ pageParam }) => {
      const service = new Service(
        new PrpcClientExt({ host, insecure }, getAccessToken),
      );
      // `method` is constrained to be a key that has an associated property of
      // type `PaginatedPrpcMethod` in a `Service`. Therefore `service[method]`
      // is guaranteed to be a `PaginatedPrpcMethod`. TSC isn't smart enough to
      // know that, so we need to use type casting.
      return await (
        service[method] as PaginatedPrpcMethod<
          PaginatedPrpcMethodRequest<S[MK]>,
          PaginatedPrpcMethodResponse<S[MK]>
        >
      )(
        { ...request, pageToken: pageParam },
        // Let react-query handle caching.
        {
          acceptCache: false,
          skipUpdate: true,
        },
      );
    },
    getNextPageParam: (lastRes) => lastRes.nextPageToken,
    ...options,
  });
}
