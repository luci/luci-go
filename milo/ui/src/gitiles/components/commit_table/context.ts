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

const RepoContext = createContext<string | undefined>(undefined);

export const RepoUrlProvider = RepoContext.Provider;

export function useRepoUrl() {
  const ctx = useContext(RepoContext);

  if (ctx === undefined) {
    throw new Error('useRepoUrl must be used within CommitTable');
  }

  return ctx;
}

const DefaultExpandedContext = createContext<boolean | undefined>(undefined);

export const DefaultExpandedProvider = DefaultExpandedContext.Provider;

export function useDefaultExpanded() {
  const ctx = useContext(DefaultExpandedContext);

  if (ctx === undefined) {
    throw new Error('useDefaultExpanded must be used within CommitTable');
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
    throw new Error('useSetDefaultExpanded must be used within CommitTable');
  }

  return ctx;
}

const ExpandedContext = createContext<boolean | undefined>(undefined);

export const ExpandedProvider = ExpandedContext.Provider;

export function useExpanded() {
  const ctx = useContext(ExpandedContext);

  if (ctx === undefined) {
    throw new Error('useExpanded must be used within CommitTableRow');
  }

  return ctx;
}

const SetExpandedContext = createContext<
  Dispatch<SetStateAction<boolean>> | undefined
>(undefined);

export const SetExpandedProvider = SetExpandedContext.Provider;

export function useSetExpanded() {
  const ctx = useContext(SetExpandedContext);

  if (ctx === undefined) {
    throw new Error('useSetExpanded must be used within CommitTable');
  }

  return ctx;
}

const CommitContext = createContext<OutputCommit | undefined>(undefined);

export const CommitProvider = CommitContext.Provider;

export function useCommit() {
  const ctx = useContext(CommitContext);

  if (ctx === undefined) {
    throw new Error('useCommit must be used within CommitTableRow');
  }

  return ctx;
}
