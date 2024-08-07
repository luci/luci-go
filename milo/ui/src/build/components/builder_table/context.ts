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

import { createContext, useContext } from 'react';

import { DecoratedClient } from '@/common/hooks/prpc_query';
import { BatchedBuildsClientImpl } from '@/proto_utils/batched_builds_client';

export const BuildsClientCtx =
  createContext<DecoratedClient<BatchedBuildsClientImpl> | null>(null);
export const NumOfBuildsCtx = createContext<number | null>(null);

export function useBuildsClient() {
  const ctx = useContext(BuildsClientCtx);
  if (ctx === null) {
    throw new Error(
      'useBuildsClient can only be used in a BuilderTableContextProvider',
    );
  }
  return ctx;
}

export function useNumOfBuilds() {
  const ctx = useContext(NumOfBuildsCtx);
  if (ctx === null) {
    throw new Error(
      'useNumOfBuilds can only be used in a BuilderTableContextProvider',
    );
  }
  return ctx;
}
