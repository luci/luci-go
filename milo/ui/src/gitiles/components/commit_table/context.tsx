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

import { SxProps, Theme } from '@mui/material';
import { createContext, Dispatch, ReactNode, SetStateAction } from 'react';

import { OutputCommit } from '@/gitiles/types';

export const RepoContext = createContext<string | undefined>(undefined);

export const TableSxContext = createContext<SxProps<Theme> | undefined>(
  undefined,
);

export const TableRowIndexContext = createContext<number | undefined>(
  undefined,
);

export const DefaultExpandedContext = createContext<boolean | undefined>(
  undefined,
);

export const TableRowPropsContext = createContext<
  { [key: string]: unknown } | undefined
>(undefined);

export const ExpandStateStoreContext = createContext<boolean[] | undefined>(
  undefined,
);

export const SetDefaultExpandedContext = createContext<
  Dispatch<SetStateAction<boolean>> | undefined
>(undefined);

export const ExpandedContext = createContext<boolean | undefined>(undefined);

export const SetExpandedContext = createContext<
  Dispatch<SetStateAction<boolean>> | undefined
>(undefined);

export const CommitContext = createContext<OutputCommit | undefined | null>(
  undefined,
);

export const RepoUrlProvider = RepoContext.Provider;

export const TableSxProvider = TableSxContext.Provider;

interface TableRowPropsProviderProps {
  readonly 'data-item-index'?: number;
  readonly children?: ReactNode;
}

export function TableRowPropsProvider({
  children,
  ...props
}: TableRowPropsProviderProps) {
  return (
    <TableRowIndexContext.Provider value={props['data-item-index']}>
      <TableRowPropsContext.Provider value={props}>
        {children}
      </TableRowPropsContext.Provider>
    </TableRowIndexContext.Provider>
  );
}

export const DefaultExpandedProvider = DefaultExpandedContext.Provider;
export const SetDefaultExpandedProvider = SetDefaultExpandedContext.Provider;

export const ExpandStateStoreProvider = ExpandStateStoreContext.Provider;
export const ExpandedProvider = ExpandedContext.Provider;
export const SetExpandedProvider = SetExpandedContext.Provider;

export const CommitProvider = CommitContext.Provider;
