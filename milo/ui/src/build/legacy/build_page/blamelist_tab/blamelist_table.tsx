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

import { useMemo } from 'react';
import { useLocalStorage } from 'react-use';

import {
  AuthorContentCell,
  AuthorHeadCell,
  CommitTableRow,
  IdContentCell,
  IdHeadCell,
  NumContentCell,
  NumHeadCell,
  TimeContentCell,
  TimeHeadCell,
  TitleContentCell,
  TitleHeadCell,
  ToggleContentCell,
  ToggleHeadCell,
  VirtualizedCommitTable,
} from '@/gitiles/components/commit_table';

import { OutputQueryBlamelistResponse } from './types';

const BLAMELIST_TABLE_DEFAULT_EXPANDED_KEY = 'blamelist-table-default-expanded';

export interface BlamelistTableProps {
  readonly repoUrl: string;
  readonly pages: readonly OutputQueryBlamelistResponse[];
}

export function BlamelistTable({ repoUrl, pages }: BlamelistTableProps) {
  const commits = useMemo(() => pages.flatMap((p) => p.commits), [pages]);
  const [defaultExpanded = false, setDefaultExpanded] =
    useLocalStorage<boolean>(BLAMELIST_TABLE_DEFAULT_EXPANDED_KEY);

  return (
    <VirtualizedCommitTable
      repoUrl={repoUrl}
      initDefaultExpanded={defaultExpanded}
      onDefaultExpandedChanged={(expand) => setDefaultExpanded(expand)}
      totalCount={commits.length}
      useWindowScroll
      fixedHeaderContent={() => (
        <>
          <ToggleHeadCell hotkey="x" />
          <NumHeadCell />
          <IdHeadCell />
          <AuthorHeadCell />
          <TimeHeadCell />
          <TitleHeadCell />
        </>
      )}
      itemContent={(i) => (
        <CommitTableRow key={commits[i].id} commit={commits[i]}>
          <ToggleContentCell />
          <NumContentCell num={i + 1} />
          <IdContentCell />
          <AuthorContentCell />
          <TimeContentCell />
          <TitleContentCell />
        </CommitTableRow>
      )}
    />
  );
}
