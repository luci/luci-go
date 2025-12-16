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

import { CircularProgress, Link, TableCell } from '@mui/material';
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
  TitleHeadCell,
  ToggleContentCell,
  ToggleHeadCell,
} from '@/gitiles/components/commit_table';
import { useCommit } from '@/gitiles/components/commit_table/context';
import { getGitilesCommitURL } from '@/gitiles/tools/utils';
import { Analysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { GitilesCommit } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { OutputQueryBlamelistResponse } from './types';

const BLAMELIST_TABLE_DEFAULT_EXPANDED_KEY = 'blamelist-table-default-expanded';

function StyledTitleContentCell({ isReverted }: { isReverted: boolean }) {
  const commit = useCommit();
  const title = commit?.message.split('\n', 1)[0] || '';
  return (
    <TableCell
      sx={{
        textDecoration: isReverted ? 'line-through' : 'none',
        whiteSpace: 'break-spaces',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
      }}
    >
      {title}
    </TableCell>
  );
}

interface CommitTablePageProps {
  readonly prevCommitCount: number;
  readonly page: OutputQueryBlamelistResponse;
  readonly suspect?: GenericSuspect;
  readonly bbid?: string;
  readonly revertedCommitsMap: ReadonlyMap<string, GitilesCommit>;
  readonly isLoadingReverts?: boolean;
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
  revertedCommitsMap,
  isLoadingReverts,
}: CommitTablePageProps) {
  return (
    <>
      {page.commits.map((commit, i) => {
        const isSuspect = suspect?.commit?.id === commit.id;
        const revertingCommit = revertedCommitsMap.get(commit.id);
        const isReverted = Boolean(revertingCommit);
        return (
          <CommitTableRow key={i} commit={commit} isReverted={isReverted}>
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
            <StyledTitleContentCell isReverted={isReverted} />
            <TableCell sx={{ textDecoration: 'none' }}>
              {isLoadingReverts ? (
                <CircularProgress size={16} />
              ) : (
                revertingCommit && (
                  <Link
                    href={getGitilesCommitURL(revertingCommit)}
                    target="_blank"
                    rel="noopener"
                  >
                    {revertingCommit.id.slice(0, 7)}
                  </Link>
                )
              )}
            </TableCell>
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
  readonly revertedCommitsMap: ReadonlyMap<string, GitilesCommit>;
  readonly isLoadingReverts?: boolean;
}

export function BlamelistTable({
  repoUrl,
  pages,
  analysis,
  revertedCommitsMap,
  isLoadingReverts,
}: BlamelistTableProps) {
  const [defaultExpanded = false, setDefaultExpanded] =
    useLocalStorage<boolean>(BLAMELIST_TABLE_DEFAULT_EXPANDED_KEY);

  let commitCount = 0;
  let suspect: GenericSuspect | undefined;
  // Show GenAI culprit if it is confirmed
  if (
    analysis?.genAiResult?.suspect?.verificationDetails?.status ===
    'Confirmed Culprit'
  ) {
    suspect = analysis?.genAiResult?.suspect
      ? GenericSuspect.fromGenAi(analysis?.genAiResult?.suspect)
      : undefined;
  } else if (
    // otherwise show Nth Section culprit if it is confirmed
    analysis?.nthSectionResult?.suspect?.verificationDetails?.status ===
    'Confirmed Culprit'
  ) {
    suspect = analysis?.nthSectionResult?.suspect
      ? GenericSuspect.fromNthSection(analysis?.nthSectionResult?.suspect)
      : undefined;
  } else {
    // otherwise show gen AI suspect.
    suspect = analysis?.genAiResult?.suspect
      ? GenericSuspect.fromGenAi(analysis?.genAiResult?.suspect)
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
        <TableCell>Reverted By</TableCell>
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
              revertedCommitsMap={revertedCommitsMap}
              isLoadingReverts={isLoadingReverts}
            />
          );
        })}
      </CommitTableBody>
    </CommitTable>
  );
}
