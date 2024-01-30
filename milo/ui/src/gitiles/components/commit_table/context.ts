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

import { OutputCommit } from '@/gitiles/types';

const RepoContext = createContext<string | null>(null);

export const RepoUrlProvider = RepoContext.Provider;

export function useRepoUrl() {
  const ctx = useContext(RepoContext);

  if (ctx === null) {
    throw new Error('useRepoUrl must be used within CommitTable');
  }

  return ctx;
}

const DefaultExpandedStateContext = createContext<
  readonly [boolean, Dispatch<SetStateAction<boolean>>] | null
>(null);

export const DefaultExpandedStateProvider =
  DefaultExpandedStateContext.Provider;

export function useDefaultExpandedState() {
  const ctx = useContext(DefaultExpandedStateContext);

  if (ctx === null) {
    throw new Error('useDefaultExpandedState must be used within CommitTable');
  }

  return ctx;
}

const CommitContext = createContext<OutputCommit | null>(null);

export const CommitProvider = CommitContext.Provider;

export function useCommit() {
  const ctx = useContext(CommitContext);

  if (ctx === null) {
    throw new Error('useCommit must be used within CommitTableRow');
  }

  return ctx;
}
