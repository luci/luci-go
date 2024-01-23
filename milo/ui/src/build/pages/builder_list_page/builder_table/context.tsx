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

import { ReactNode, createContext, useContext } from 'react';

import {
  DecoratedClient,
  usePrpcServiceClient,
} from '@/common/hooks/prpc_query';
import { BatchedBuildsClientImpl } from '@/proto_utils/batched_builds_client';

const BuildsClientCtx =
  createContext<DecoratedClient<BatchedBuildsClientImpl> | null>(null);
const NumOfBuildsCtx = createContext<number | null>(null);

export interface BuilderTableContextProviderProps {
  readonly numOfBuilds: number;
  readonly children: ReactNode;
}

export function BuilderTableContextProvider({
  numOfBuilds,
  children,
}: BuilderTableContextProviderProps) {
  const buildsClient = usePrpcServiceClient({
    host: SETTINGS.buildbucket.host,
    ClientImpl: BatchedBuildsClientImpl,
  });

  return (
    <BuildsClientCtx.Provider value={buildsClient}>
      <NumOfBuildsCtx.Provider value={numOfBuilds}>
        {children}
      </NumOfBuildsCtx.Provider>
    </BuildsClientCtx.Provider>
  );
}

export function useBuildsClient() {
  const ctx = useContext(BuildsClientCtx);
  if (ctx === null) {
    throw new Error(
      'useBuildsClient can only be used within BuilderTableContextProvider',
    );
  }
  return ctx;
}

export function useNumOfBuilds() {
  const ctx = useContext(NumOfBuildsCtx);
  if (ctx === null) {
    throw new Error(
      'useNumOfBuilds can only be used within BuilderTableContextProvider',
    );
  }
  return ctx;
}
