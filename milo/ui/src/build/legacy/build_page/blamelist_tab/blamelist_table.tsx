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

import { memo } from 'react';
import { useLocalStorage } from 'react-use';

import {
  AuthorContentCell,
  AuthorHeadCell,
  CommitTable,
  CommitTableHead,
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
} from '@/gitiles/components/commit_table';
import { CommitTableBody } from '@/gitiles/components/commit_table/commit_table_body';

import { OutputQueryBlamelistResponse } from './types';

const BLAMELIST_TABLE_DEFAULT_EXPANDED_KEY = 'blamelist-table-default-expanded';

interface CommitTablePageProps {
  readonly prevCommitCount: number;
  readonly page: OutputQueryBlamelistResponse;
}

// The table body can be large and expensive to render.
// Use `memo` to stop it from rerendering when only the default expansion state
// were changed.
//
// Group rows by pages so when new pages are loaded, we don't need to rerender
// everything.
const CommitTablePage = memo(function CommitTablePage({
  prevCommitCount,
  page,
}: CommitTablePageProps) {
  return (
    <>
      {page.commits.map((commit, i) => (
        <CommitTableRow key={i} commit={commit}>
          <ToggleContentCell />
          <NumContentCell num={prevCommitCount + i + 1} />
          <IdContentCell />
          <AuthorContentCell />
          <TimeContentCell />
          <TitleContentCell />
        </CommitTableRow>
      ))}
    </>
  );
});

export interface BlamelistTableProps {
  readonly repoUrl: string;
  readonly pages: readonly OutputQueryBlamelistResponse[];
}

export function BlamelistTable({ repoUrl, pages }: BlamelistTableProps) {
  const [defaultExpanded = false, setDefaultExpanded] =
    useLocalStorage<boolean>(BLAMELIST_TABLE_DEFAULT_EXPANDED_KEY);

  let commitCount = 0;
  return (
    <CommitTable
      repoUrl={repoUrl}
      initDefaultExpanded={defaultExpanded}
      onDefaultExpandedChanged={(expand) => setDefaultExpanded(expand)}
    >
      <CommitTableHead>
        <ToggleHeadCell hotkey="x" />
        <NumHeadCell />
        <IdHeadCell />
        <AuthorHeadCell />
        <TimeHeadCell />
        <TitleHeadCell />
      </CommitTableHead>
      <CommitTableBody>
        {pages.map((page, i) => {
          const prevCount = commitCount;
          commitCount += page.commits.length;
          return (
            <CommitTablePage key={i} page={page} prevCommitCount={prevCount} />
          );
        })}
      </CommitTableBody>
    </CommitTable>
  );
}
