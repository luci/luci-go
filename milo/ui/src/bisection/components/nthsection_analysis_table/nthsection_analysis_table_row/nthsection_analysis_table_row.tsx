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

import './nthsection_analysis_table_row.css';

import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';

import { RerunStatusInfo } from '@/bisection/components/status_info';
import { linkToBuild, linkToCommit } from '@/bisection/tools/link_constructors';
import { getFormattedTimestamp } from '@/bisection/tools/timestamp_formatters';
import { SingleRerun } from '@/common/services/luci_bisection';

interface Props {
  rerun: SingleRerun;
}

// Display the default rerun type when rerun type information in missing.
// Test analysis rpc doesn't return the rerun type because all rerun are "Nthsection" type.
const DEFAULT_RERUN_TYPE = 'Nthsection';

export function NthSectionAnalysisTableRow({ rerun }: Props) {
  const { startTime, endTime, bbid, commit, index, type } = rerun;

  const buildLink = linkToBuild(bbid);
  const commitLink = linkToCommit(commit);
  return (
    <>
      <TableRow data-testid="nthsection_analysis_table_row">
        {/* Commit position is filled either for all reruns or no reruns of an analysis.
            If no commit position is available, rerun index is used instead. */}
        <TableCell>{commit.position || index}</TableCell>
        <TableCell>
          {/* TODO (nqmtuan): Show review title instead */}
          <Link
            href={commitLink.url}
            target="_blank"
            rel="noreferrer"
            underline="always"
          >
            {commitLink.linkText}
          </Link>
        </TableCell>
        <TableCell>
          <RerunStatusInfo
            status={rerun.rerunResult.rerunStatus}
          ></RerunStatusInfo>
        </TableCell>
        <TableCell>
          <Link
            href={buildLink.url}
            target="_blank"
            rel="noreferrer"
            underline="always"
          >
            {buildLink.linkText}
          </Link>
        </TableCell>
        <TableCell>{type || DEFAULT_RERUN_TYPE}</TableCell>
        <TableCell>{getFormattedTimestamp(startTime)}</TableCell>
        <TableCell>{getFormattedTimestamp(endTime)}</TableCell>
      </TableRow>
    </>
  );
}
