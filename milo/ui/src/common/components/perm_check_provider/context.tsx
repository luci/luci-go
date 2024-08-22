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

import { createContext, ReactNode } from 'react';

import { useBatchedMiloInternalClient } from '@/common/hooks/prpc_clients';
import { DecoratedClient } from '@/common/hooks/prpc_query';
import { BatchedMiloInternalClientImpl } from '@/proto_utils/batched_milo_internal_client';

export const ClientCtx =
  createContext<DecoratedClient<BatchedMiloInternalClientImpl> | null>(null);

export interface PermCheckProviderProps {
  readonly children: ReactNode;
}

export function PermCheckProvider({ children }: PermCheckProviderProps) {
  // Use a single client instance so all requests in the same rendering cycle
  // can be batched together.
  const client = useBatchedMiloInternalClient();

  return <ClientCtx.Provider value={client}>{children}</ClientCtx.Provider>;
}
