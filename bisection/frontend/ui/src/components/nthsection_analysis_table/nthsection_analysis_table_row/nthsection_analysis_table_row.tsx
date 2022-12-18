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

import './nthsection_analysis_table_row.css';

import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';

import { linkToBuild, linkToCommit } from '../../../tools/link_constructors';
import { getFormattedTimestamp } from '../../../tools/timestamp_formatters';
import { RerunStatusInfo } from '../../status_info/status_info';
import { SingleRerun } from '../../../services/luci_bisection';

interface Props {
  rerun: SingleRerun;
}

export const NthSectionAnalysisTableRow = ({ rerun }: Props) => {
  const {
    startTime,
    endTime,
    bbid,
    rerunResult,
    commit,
    index,
  } = rerun;

  const buildLink = linkToBuild(bbid);
  const commitLink = linkToCommit(commit)
  return (
    <>
      <TableRow data-testid='nthsection_analysis_table_row'>
        <TableCell>
          {index}
        </TableCell>
        <TableCell>
          {/* TODO (nqmtuan): Show review title instead */}
          <Link
            href={commitLink.url}
            target='_blank'
            rel='noreferrer'
            underline='always'
          >
            {commitLink.linkText}
          </Link>
        </TableCell>
        <TableCell>
          <RerunStatusInfo status={rerunResult.rerunStatus}></RerunStatusInfo>
        </TableCell>
        <TableCell>
          <Link
            href={buildLink.url}
            target='_blank'
            rel='noreferrer'
            underline='always'
          >
            {buildLink.linkText}
          </Link>
        </TableCell>
        <TableCell>
          {getFormattedTimestamp(startTime)}
        </TableCell>
        <TableCell>
          {getFormattedTimestamp(endTime)}
        </TableCell>
      </TableRow>
    </>
  );
};
