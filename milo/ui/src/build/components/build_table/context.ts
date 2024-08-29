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

import { Dispatch, SetStateAction, createContext, useContext } from 'react';

import { OutputBuild } from '@/build/types';

const DefaultExpandedContext = createContext<boolean | undefined>(undefined);

export const DefaultExpandedProvider = DefaultExpandedContext.Provider;

export function useDefaultExpanded() {
  const ctx = useContext(DefaultExpandedContext);

  if (ctx === undefined) {
    throw new Error('useDefaultExpanded can only be used in a BuildTable');
  }

  return ctx;
}

const SetDefaultExpandedContext = createContext<
  Dispatch<SetStateAction<boolean>> | undefined
>(undefined);

export const SetDefaultExpandedProvider = SetDefaultExpandedContext.Provider;

export function useSetDefaultExpanded() {
  const ctx = useContext(SetDefaultExpandedContext);

  if (ctx === undefined) {
    throw new Error('useSetDefaultExpanded can only be used in a BuildTable');
  }

  return ctx;
}

const BuildContext = createContext<OutputBuild | undefined>(undefined);

export const BuildProvider = BuildContext.Provider;

export function useBuild() {
  const ctx = useContext(BuildContext);

  if (ctx === undefined) {
    throw new Error('useBuild can only be used in a BuildTableRow');
  }

  return ctx;
}

const RowExpandedContext = createContext<boolean | undefined>(undefined);

export const RowExpandedProvider = RowExpandedContext.Provider;

export function useRowExpanded() {
  const ctx = useContext(RowExpandedContext);

  if (ctx === undefined) {
    throw new Error('useRowExpandedState can only be used in a BuildTableRow');
  }

  return ctx;
}

const SetRowExpandedContext = createContext<
  Dispatch<SetStateAction<boolean>> | undefined
>(undefined);

export const SetRowExpandedProvider = SetRowExpandedContext.Provider;

export function useSetRowExpanded() {
  const ctx = useContext(SetRowExpandedContext);

  if (ctx === undefined) {
    throw new Error('useSetRowExpanded can only be used in a BuildTableRow');
  }

  return ctx;
}
