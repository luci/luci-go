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

import './nthsection_analysis_table.css';

import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import { PlainTable } from '@/bisection/components/plain_table';
import { AnalysisStatusInfo } from '@/bisection/components/status_info';
import { getCommitShortHash } from '@/bisection/tools/commit_formatters';
import { EMPTY_LINK } from '@/bisection/tools/link_constructors';
import { getFormattedTimestamp } from '@/bisection/tools/timestamp_formatters';
import {
  NthSectionAnalysisResult,
  SingleRerun,
} from '@/common/services/luci_bisection';

import { NthSectionAnalysisTableRow } from './nthsection_analysis_table_row';

interface NthSectionAnalysisTableProps {
  result?: NthSectionAnalysisResult | null;
}

interface RerunProps {
  reruns: SingleRerun[];
}

export function NthSectionAnalysisTable({
  result,
}: NthSectionAnalysisTableProps) {
  if (result === null || result === undefined) {
    return (
      <span className="data-placeholder">There is no nthsection analysis</span>
    );
  }

  const reruns = result?.reruns ?? [];
  const sortedReruns = reruns.sort(
    // All reruns used in Nth section analysis should have indicies.
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    (a, b) => parseInt(a.index!) - parseInt(b.index!),
  );
  return (
    <>
      <NthSectionAnalysisDetail result={result}></NthSectionAnalysisDetail>
      <NthSectionAnalysisRerunsTable
        reruns={sortedReruns}
      ></NthSectionAnalysisRerunsTable>
    </>
  );
}

interface NthSectionAnalysisDetailProps {
  result: NthSectionAnalysisResult;
}

export function NthSectionAnalysisDetail({
  result,
}: NthSectionAnalysisDetailProps) {
  const commitLink = EMPTY_LINK;
  const suspect = result.suspect;
  if (suspect) {
    commitLink.url = suspect.reviewUrl;
    commitLink.linkText = getCommitShortHash(suspect.commit.id);
    if (suspect.reviewTitle) {
      commitLink.linkText += `: ${suspect.reviewTitle}`;
    }
  }
  return (
    <TableContainer>
      <PlainTable>
        <colgroup>
          <col style={{ width: '15%' }} />
          <col style={{ width: '35%' }} />
          <col style={{ width: '15%' }} />
          <col style={{ width: '35%' }} />
        </colgroup>
        <TableBody data-testid="nthsection-analysis-detail">
          <TableRow>
            <TableCell variant="head">Start time</TableCell>
            <TableCell>{getFormattedTimestamp(result.startTime)}</TableCell>
            <TableCell variant="head">End time</TableCell>
            <TableCell>{getFormattedTimestamp(result.endTime)}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant="head">Status</TableCell>
            <TableCell>
              <AnalysisStatusInfo status={result.status}></AnalysisStatusInfo>
            </TableCell>
            <TableCell variant="head">Suspect</TableCell>
            <TableCell>
              <Link
                href={commitLink.url}
                target="_blank"
                rel="noreferrer"
                underline="always"
              >
                {commitLink.linkText}
              </Link>
            </TableCell>
          </TableRow>
        </TableBody>
      </PlainTable>
    </TableContainer>
  );
}

export function NthSectionAnalysisRerunsTable({ reruns }: RerunProps) {
  if (!reruns || reruns.length === 0) {
    return <span className="data-placeholder">No reruns found</span>;
  }
  return (
    <TableContainer
      component={Paper}
      className="nthsection-analysis-table-container"
    >
      <Table
        className="nthsection-analysis-table"
        size="small"
        data-testid="nthsection-analysis-rerun-table"
      >
        <TableHead>
          <TableRow>
            {/* Commit position is filled either for all reruns or no reruns of an analysis.
            If no commit position is available, rerun index is used instead. */}
            {reruns[0].commit.position ? (
              <TableCell>Commit position</TableCell>
            ) : (
              <TableCell>Index</TableCell>
            )}
            <TableCell>Commit</TableCell>
            <TableCell>Status</TableCell>
            <TableCell>Run</TableCell>
            <TableCell>Type</TableCell>
            <TableCell>Start time</TableCell>
            <TableCell>End time</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {/* TODO (nqmtuan): Show the "anchors" (last passed, first failed, number of commits in between etc) */}
          {reruns.map((rerun) => (
            <NthSectionAnalysisTableRow key={rerun.bbid} rerun={rerun} />
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}
