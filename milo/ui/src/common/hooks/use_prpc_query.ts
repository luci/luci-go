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
  UseQueryOptions,
  UseQueryResult,
  useInfiniteQuery,
  useQuery,
} from '@tanstack/react-query';

import {
  useAuthState,
  useGetAccessToken,
} from '@/common/components/auth_state_provider';
import { CacheOption } from '@/generic_libs/tools/cached_fn';
import { PrpcClientExt } from '@/generic_libs/tools/prpc_client_ext';

export type PrpcMethod<Req, Ret> = (
  req: Req,
  opt?: CacheOption
) => Promise<Ret>;

export type PrpcServiceMethodKeys<S> = keyof {
  // The request type has to be `any` because the argument type must be contra-
  // variant when sub-typing a function.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  [MK in keyof S as S[MK] extends PrpcMethod<any, object> ? MK : never]: S[MK];
};

export type PrpcMethodRequest<T> = T extends PrpcMethod<infer Req, infer _Res>
  ? Req
  : never;

export type PrpcMethodResponse<T> = T extends PrpcMethod<infer _Req, infer Res>
  ? Res
  : never;

export interface UsePrpcQueryOptions<S, MK, Req, Res, TError, TData> {
  readonly host: string;
  readonly insecure?: boolean;
  readonly Service: Constructor<S, [PrpcClientExt]> & { SERVICE: string };
  readonly method: MK;
  readonly request: Req;

  /**
   * options will be passed to `useQuery` from `@tanstack/react-query`.
   */
  readonly options?: Omit<
    UseQueryOptions<Res, TError, TData, [string, string, string, MK, Req]>,
    'queryKey' | 'queryFn'
  >;
}

/**
 * Call a pRPC method via `@tanstack/react-query`.
 *
 * This hook
 *  * reduces boilerplate, and
 *  * ensures the `queryKey` is populated correctly.
 */
export function usePrpcQuery<
  S extends object,
  MK extends PrpcServiceMethodKeys<S>,
  TError = unknown,
  TData = PrpcMethodResponse<S[MK]>
>(
  opts: UsePrpcQueryOptions<
    S,
    MK,
    PrpcMethodRequest<S[MK]>,
    PrpcMethodResponse<S[MK]>,
    TError,
    TData
  >
): UseQueryResult<TData, TError> {
  const { host, insecure, Service, method, request, options } = opts;

  const { identity } = useAuthState();
  const getAccessToken = useGetAccessToken();
  return useQuery({
    queryKey: [
      // The query response is tied to the user identity (ACL). The user
      // identity may change after a auth state refresh after user logs in/out
      // in another browser tab.
      identity,
      // Some pRPC services may get hosted on multiple hosts (e.g. swarming).
      // Ensure the query to one host is not reused by query to another host.
      host,
      // Ensure methods sharing the same name from different services won't
      // cause collision.
      Service.SERVICE,
      // Obviously query to one method shall not be reused by query to another
      // method.
      method,
      // Include the whole request so whenever the request is changed, a new
      // RPC call is triggered.
      request,
    ],
    queryFn: async () => {
      const service = new Service(
        new PrpcClientExt({ host, insecure }, getAccessToken)
      );
      // `method` is constrained to be a key that has an associated property of
      // type `PrpcMethod` in a `Service`. Therefore `service[method]` is
      // guaranteed to be a `PrpcMethod`. TSC isn't smart enough to know that,
      // so we need to use type casting.
      return await (
        service[method] as PrpcMethod<
          PrpcMethodRequest<S[MK]>,
          PrpcMethodResponse<S[MK]>
        >
      )(
        request,
        // Let react-query handle caching.
        {
          acceptCache: false,
          skipUpdate: true,
        }
      );
    },
    ...options,
  });
}

export type PaginatedPrpcMethod<
  Req extends { pageToken?: string },
  Ret extends { nextPageToken?: string }
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

export interface UseInfinitePrpcQueryOptions<S, MK, Req, Res, TError, TData> {
  readonly host: string;
  readonly insecure?: boolean;
  readonly Service: Constructor<S, [PrpcClientExt]> & { SERVICE: string };
  readonly method: MK;
  readonly request: Req;

  /**
   * options will be passed to `useInfiniteQuery` from `@tanstack/react-query`.
   */
  readonly options?: Omit<
    UseInfiniteQueryOptions<
      Res,
      TError,
      TData,
      Res,
      [string, string, string, MK, Req]
    >,
    'queryKey' | 'queryFn' | 'getNextPageParam'
  >;
}

/**
 * Call a pRPC method via `@tanstack/react-query`.
 *
 * This hook
 *  * reduces boilerplate, and
 *  * ensures the `queryKey` is populated correctly.
 */
// For the same reason that `@tanstack/react-query` maintains a separate set of
// functions/interfaces for `useInfiniteQuery`, it's unlikely that we can dedupe
// this function (and its supporting types) with `usePrpcQuery` in a meaningful
// way.
export function useInfinitePrpcQuery<
  S extends object,
  MK extends PrpcServicePaginatedMethodKeys<S>,
  TError = unknown,
  TData = PaginatedPrpcMethodResponse<S[MK]>
>(
  opts: UseInfinitePrpcQueryOptions<
    S,
    MK,
    PaginatedPrpcMethodRequest<S[MK]>,
    PaginatedPrpcMethodResponse<S[MK]>,
    TError,
    TData
  >
): UseInfiniteQueryResult<TData, TError> {
  const { host, insecure, Service, method, request, options } = opts;

  const { identity } = useAuthState();
  const getAccessToken = useGetAccessToken();
  return useInfiniteQuery({
    queryKey: [
      // The query response is tied to the user identity (ACL). The user
      // identity may change after a auth state refresh after user logs in/out
      // in another browser tab.
      identity,
      // Some pRPC services may get hosted on multiple hosts (e.g. swarming).
      // Ensure the query to one host is not reused by query to another host.
      host,
      // Ensure methods sharing the same name from different services won't
      // cause collision.
      Service.SERVICE,
      // Obviously query to one method shall not be reused by query to another
      // method.
      method,
      // Include the whole request so whenever the request is changed, a new
      // RPC call is triggered.
      request,
    ],
    queryFn: async ({ pageParam }) => {
      const service = new Service(
        new PrpcClientExt({ host, insecure }, getAccessToken)
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
        }
      );
    },
    getNextPageParam: (lastRes) => lastRes.nextPageToken,
    ...options,
  });
}
