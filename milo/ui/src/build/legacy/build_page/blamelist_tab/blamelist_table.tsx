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

import { GenericSuspect } from '@/bisection/types';
import {
  AnalysisContentCell,
  AnalysisHeadCell,
  AuthorContentCell,
  AuthorHeadCell,
  CommitTable,
  CommitTableBody,
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
import { Analysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

import { OutputQueryBlamelistResponse } from './types';

const BLAMELIST_TABLE_DEFAULT_EXPANDED_KEY = 'blamelist-table-default-expanded';

interface CommitTablePageProps {
  readonly prevCommitCount: number;
  readonly page: OutputQueryBlamelistResponse;
  readonly suspect?: GenericSuspect;
  readonly bbid?: string;
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
  suspect,
  bbid,
}: CommitTablePageProps) {
  return (
    <>
      {page.commits.map((commit, i) => {
        const isSuspect = suspect?.commit?.id === commit.id;
        return (
          <CommitTableRow key={i} commit={commit}>
            <ToggleContentCell />
            <NumContentCell num={prevCommitCount + i + 1} />
            {suspect && (
              <AnalysisContentCell
                suspect={isSuspect ? suspect : undefined}
                bbid={bbid}
              />
            )}
            <IdContentCell />
            <AuthorContentCell />
            <TimeContentCell />
            <TitleContentCell />
          </CommitTableRow>
        );
      })}
    </>
  );
});

export interface BlamelistTableProps {
  readonly repoUrl: string;
  readonly pages: readonly OutputQueryBlamelistResponse[];
  readonly analysis?: Analysis;
}

export function BlamelistTable({
  repoUrl,
  pages,
  analysis,
}: BlamelistTableProps) {
  const [defaultExpanded = false, setDefaultExpanded] =
    useLocalStorage<boolean>(BLAMELIST_TABLE_DEFAULT_EXPANDED_KEY);

  let commitCount = 0;
  const verificationStates = [
    'Confirmed Culprit',
    'Verification Scheduled',
    'Under Verification',
  ];

  let suspect: GenericSuspect | undefined;
  // GenAI suspect is generally identified and verified well before the
  // nthsection so we use that unless verfication vindicates it.
  if (
    verificationStates.includes(
      analysis?.genAiResult?.suspect?.verificationDetails?.status || '',
    )
  ) {
    suspect = analysis?.genAiResult?.suspect
      ? GenericSuspect.fromGenAi(analysis?.genAiResult?.suspect)
      : undefined;
  } else if (
    verificationStates.includes(
      analysis?.nthSectionResult?.suspect?.verificationDetails?.status || '',
    )
  ) {
    suspect = analysis?.nthSectionResult?.suspect
      ? GenericSuspect.fromNthSection(analysis?.nthSectionResult?.suspect)
      : undefined;
  }
  return (
    <CommitTable
      repoUrl={repoUrl}
      initDefaultExpanded={defaultExpanded}
      onDefaultExpandedChanged={(expand) => setDefaultExpanded(expand)}
    >
      <CommitTableHead>
        <ToggleHeadCell hotkey="x" />
        <NumHeadCell />
        {suspect && <AnalysisHeadCell />}
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
            <CommitTablePage
              key={i}
              page={page}
              prevCommitCount={prevCount}
              suspect={suspect}
              bbid={analysis?.firstFailedBbid}
            />
          );
        })}
      </CommitTableBody>
    </CommitTable>
  );
}
