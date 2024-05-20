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

import { OutputChangepointGroupSummary } from '@/analysis/types';

import { GetDetailsUrlPath } from './types';

const Ctx = createContext<GetDetailsUrlPath | null>(null);

export interface RegressionTableContextProviderProps {
  readonly getDetailsUrlPath: GetDetailsUrlPath;
  readonly children: ReactNode;
}

export function RegressionTableContextProvider({
  getDetailsUrlPath,
  children,
}: RegressionTableContextProviderProps) {
  return <Ctx.Provider value={getDetailsUrlPath}>{children}</Ctx.Provider>;
}

export function useDetailsUrlPath(regression: OutputChangepointGroupSummary) {
  const ctx = useContext(Ctx);
  if (ctx === null) {
    throw new Error(
      'useDetailsUrlPath can only be used within RegressionTable',
    );
  }
  return ctx(regression);
}
