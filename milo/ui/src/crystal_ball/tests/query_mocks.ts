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

import { UseQueryResult } from '@tanstack/react-query';

/**
 * Creates a mock UseQueryResult representing an error state.
 */
export function createMockErrorResult<T>(error: Error): UseQueryResult<T> {
  return {
    data: undefined,
    dataUpdatedAt: 0,
    error,
    errorUpdateCount: 1,
    errorUpdatedAt: 0,
    failureCount: 1,
    failureReason: error,
    fetchStatus: 'idle',
    isEnabled: true,
    isError: true,
    isFetched: true,
    isFetchedAfterMount: true,
    isFetching: false,
    isInitialLoading: false,
    isLoading: false,
    isLoadingError: true,
    isPaused: false,
    isPending: false,
    isPlaceholderData: false,
    isRefetchError: false,
    isRefetching: false,
    isStale: false,
    isSuccess: false,
    promise: (() => {
      const p = Promise.reject(error);
      p.catch(() => {}); // Prevents unhandled promise rejection crash in Jest workers
      return p;
    })(),
    refetch: jest.fn(),
    status: 'error',
  };
}

/**
 * Creates a mock UseQueryResult representing a pending (loading) state.
 */
export function createMockPendingResult<T>(): UseQueryResult<T> {
  return {
    data: undefined,
    dataUpdatedAt: 0,
    error: null,
    errorUpdateCount: 0,
    errorUpdatedAt: 0,
    failureCount: 0,
    failureReason: null,
    fetchStatus: 'fetching',
    isEnabled: true,
    isError: false,
    isFetched: false,
    isFetchedAfterMount: false,
    isFetching: true,
    isInitialLoading: true,
    isLoading: true,
    isLoadingError: false,
    isPaused: false,
    isPending: true,
    isPlaceholderData: false,
    isRefetchError: false,
    isRefetching: false,
    isStale: true,
    isSuccess: false,
    promise: new Promise(() => {}), // Never resolves, simulating infinite load
    refetch: jest.fn(),
    status: 'pending',
  };
}

/**
 * Creates a mock UseQueryResult with default values to satisfy TypeScript
 * without type casting.
 */
export function createMockQueryResult<T>(data: T): UseQueryResult<T> {
  return {
    data,
    dataUpdatedAt: 0,
    error: null,
    errorUpdateCount: 0,
    errorUpdatedAt: 0,
    failureCount: 0,
    failureReason: null,
    fetchStatus: 'idle',
    isEnabled: true,
    isError: false,
    isFetched: true,
    isFetchedAfterMount: true,
    isFetching: false,
    isInitialLoading: false,
    isLoading: false,
    isLoadingError: false,
    isPaused: false,
    isPending: false,
    isPlaceholderData: false,
    isRefetchError: false,
    isRefetching: false,
    isStale: false,
    isSuccess: true,
    promise: Promise.resolve(data),
    refetch: jest.fn(),
    status: 'success',
  };
}
