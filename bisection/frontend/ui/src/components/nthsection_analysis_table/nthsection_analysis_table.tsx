// Copyright 2022 The LUCI Authors.
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

import { linkToCommit, EMPTY_LINK } from '../../tools/link_constructors';
import { getFormattedTimestamp } from '../../tools/timestamp_formatters';

import { AnalysisStatusInfo } from '../status_info/status_info';
import { PlainTable } from '../plain_table/plain_table';
import { NthSectionAnalysisTableRow } from './nthsection_analysis_table_row/nthsection_analysis_table_row';

import {
  NthSectionAnalysisResult,
  SingleRerun,
} from '../../services/luci_bisection';

interface Props {
  result?: NthSectionAnalysisResult | null;
}

interface RerunProps {
  reruns: SingleRerun[];
}

function getRows(reruns: SingleRerun[]) {
  return reruns.map((rerun) => (
    <NthSectionAnalysisTableRow
      key={rerun.commit.id}
      rerun={rerun}
    />
  ));
}

export const NthSectionAnalysisTable = ({ result }: Props) => {
  if (result == null || result == undefined) {
    return <>There is no nthsection analysis</>;
  }

  const reruns = result?.reruns ?? [];
  const sortedReruns = reruns.sort((a, b) => parseInt(a.index!) - parseInt(b.index!));
  return (
    <>
    <NthSectionAnalysisDetail result={result}></NthSectionAnalysisDetail>
    <NthSectionAnalysisRerunsTable reruns={sortedReruns}></NthSectionAnalysisRerunsTable>
    </>
  );
};

export const NthSectionAnalysisDetail = ({ result }: Props) => {
  var commitLink = EMPTY_LINK
  if (result?.suspect) {
    commitLink = linkToCommit(result.suspect)
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
        <TableBody data-testid='nthsection-analysis-detail'>
          <TableRow>
            <TableCell variant='head'>Start time</TableCell>
            <TableCell>{getFormattedTimestamp(result!.startTime)}</TableCell>
            <TableCell variant='head'>End time</TableCell>
            <TableCell>{getFormattedTimestamp(result!.endTime)}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant='head'>Status</TableCell>
            <TableCell>
              <AnalysisStatusInfo status={result!.status}></AnalysisStatusInfo>
            </TableCell>
            <TableCell variant='head'>Suspect</TableCell>
            {/* TODO (nqmtuan): Show review title instead */}
            <TableCell>
              <Link
                href={commitLink.url}
                target='_blank'
                rel='noreferrer'
                underline='always'
              >
                {commitLink.linkText}
              </Link>
            </TableCell>
          </TableRow>
        </TableBody>
      </PlainTable>
    </TableContainer>
  );
};

export const NthSectionAnalysisRerunsTable = ({ reruns }: RerunProps) => {
  if (!reruns) {
    return <>Could not find any reruns</>;
  }
  return (
    <TableContainer component={Paper} className='nthsection-analysis-table-container'>
      <Table className='nthsection-analysis-table' size='small' data-testid='nthsection-analysis-rerun-table'>
        <TableHead>
          <TableRow>
            <TableCell>Index</TableCell>
            <TableCell>Commit</TableCell>
            <TableCell>Status</TableCell>
            <TableCell>Run</TableCell>
            <TableCell>Start time</TableCell>
            <TableCell>End time</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {/* TODO (nqmtuan): Show the "anchors" (last passed, first failed, number of commits in between etc) */}
          {getRows(reruns)}
        </TableBody>
      </Table>
    </TableContainer>
  );
}