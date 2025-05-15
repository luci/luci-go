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

import {
  UseInfiniteQueryOptions,
  UseQueryOptions,
  QueryKey,
  DefaultError,
  InfiniteData,
} from '@tanstack/react-query';

/**
 * Represents the options that can be passed to custom hooks wrapping `useQuery`.
 *
 * This type excludes `queryKey` and `queryFn`, as these are typically managed
 * internally by the wrapper hook based on its specific inputs (e.g., request data).
 *
 * It allows consumers of the wrapper hook to configure behavior like caching (`gcTime`, `staleTime`),
 * retries, `enabled` state, data transformations (`select`), etc.
 *
 * @template TQueryFnData - The type of data returned by the underlying query function.
 * @template TError - The type of error the query function might throw. Defaults to `Error`.
 * @template TData - The type of data returned by the `useQuery` hook after transformations (e.g., `select`). Defaults to `TQueryFnData`.
 */
export type WrapperQueryOptions<
  TQueryFnData = unknown,
  TError = Error,
  TData = TQueryFnData,
> = Omit<
  // Start with the standard options for useQuery
  UseQueryOptions<TQueryFnData, TError, TData, QueryKey>,
  // Exclude the properties that the wrapper hook is responsible for defining
  'queryKey' | 'queryFn'
>;

/**
 * WORKAROUND APPROACH (If Step 2 fails)
 * Uses 'any' for TPageParam internally within Omit to bypass faulty constraint checks.
 * The outer TPageParam generic remains for external type safety.
 *
 * @template TQueryFnData - Data type for a single page.
 * @template TError - Error type.
 * @template TData - Data type returned by the hook (usually InfiniteData).
 * @template TQueryKey - Query key type.
 * @template TPageParam - Page parameter type.
 */

export type WrapperInfiniteQueryOptions<
  TQueryFnData = unknown,
  TError = DefaultError,
  TData = InfiniteData<TQueryFnData>,
  TQueryData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
  TPageParam = unknown, // <-- Keep outer TPageParam correct
> = Omit<
  // Use 'any' ONLY for TPageParam inside UseInfiniteQueryOptions for Omit
  UseInfiniteQueryOptions<
    TQueryFnData,
    TError,
    TData,
    TQueryData,
    TQueryKey,
    TPageParam
  >,
  // Exclude properties managed by the wrapper hook:
  | 'queryKey'
  | 'queryFn'
  | 'initialPageParam'
  | 'getNextPageParam'
  | 'getPreviousPageParam'
>;
