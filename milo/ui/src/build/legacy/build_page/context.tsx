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

import { ReactNode, createContext, useContext } from 'react';

import { Build } from '@/common/services/buildbucket';

// Use `undefined` to denote missing context provider.
// `null` means there's no build.
const BuildCtx = createContext<Build | null | undefined>(undefined);

export interface BuildProviderProps {
  readonly build: Build | null;
  readonly children: ReactNode;
}

export function BuildContextProvider({ build, children }: BuildProviderProps) {
  return <BuildCtx.Provider value={build}>{children}</BuildCtx.Provider>;
}

export function useBuild() {
  const ctx = useContext(BuildCtx);
  if (ctx === undefined) {
    throw new Error('useBuild can only be used in a BuildContextProvider');
  }
  return ctx;
}
