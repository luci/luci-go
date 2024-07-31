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

import { useQuery } from '@tanstack/react-query';
import { createContext, ReactNode, useContext } from 'react';

import {
  DecoratedClient,
  usePrpcServiceClient,
} from '@/common/hooks/prpc_query';
import { BatchedMiloInternalClientImpl } from '@/proto_utils/batched_milo_internal_client';

const ClientCtx =
  createContext<DecoratedClient<BatchedMiloInternalClientImpl> | null>(null);

export interface PermCheckProviderProps {
  readonly children: ReactNode;
}

export function PermCheckProvider({ children }: PermCheckProviderProps) {
  // Use a single client instance so all requests in the same rendering cycle
  // can be batched together.
  const host = SETTINGS.milo.host;
  const isLoopback =
    host === 'localhost' ||
    host.startsWith('localhost:') ||
    host === '127.0.0.1' ||
    host.startsWith('127.0.0.1:');
  const useInsecure = isLoopback && document.location.protocol === 'http:';
  const client = usePrpcServiceClient({
    host: host,
    insecure: useInsecure,
    ClientImpl: BatchedMiloInternalClientImpl,
  });

  return <ClientCtx.Provider value={client}>{children}</ClientCtx.Provider>;
}

/**
 * Checks whether the user has permission `perm` in realm `realm`.
 *
 * All permission checks in the same rendering cycle are batched into a single
 * RPC request and the results are cached by react-query.
 *
 * When either `perm` or `realm` is not specified, the query will not be sent.
 */
export function usePermCheck(
  realm?: string | null,
  perm?: string | null,
): [allowed: boolean, isLoading: boolean] {
  const client = useContext(ClientCtx);
  if (!client) {
    throw new Error('usePermCheck must be used within PermCheckProvider');
  }

  const { data, isError, error, isLoading } = useQuery({
    ...client.BatchCheckPermissions.query({
      realm: realm!,
      permissions: [perm!],
    }),
    select(data) {
      return data.results[perm!];
    },
    enabled: Boolean(realm) && Boolean(perm),
  });
  if (isError) {
    throw error;
  }
  return [data ?? false, isLoading];
}
