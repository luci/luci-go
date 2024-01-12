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

import './heuristic_analysis_table_row.css';

import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import { nanoid } from 'nanoid';

import { getCommitShortHash } from '@/bisection/tools/commit_formatters';
import { HeuristicSuspect } from '@/common/services/luci_bisection';

interface Props {
  suspect: HeuristicSuspect;
}

export function HeuristicAnalysisTableRow({ suspect }: Props) {
  const {
    gitilesCommit,
    reviewUrl,
    reviewTitle,
    justification,
    score,
    confidenceLevel,
  } = suspect;

  const reasons = justification.split('\n');
  const reasonCount = reasons.length;

  let suspectDescription = getCommitShortHash(gitilesCommit.id);
  if (reviewTitle) {
    suspectDescription += `: ${reviewTitle}`;
  }

  return (
    <>
      <TableRow data-testid="heuristic_analysis_table_row">
        <TableCell rowSpan={reasonCount} className="overview-cell">
          <Link
            href={reviewUrl}
            target="_blank"
            rel="noreferrer"
            underline="always"
          >
            {suspectDescription}
          </Link>
        </TableCell>
        <TableCell rowSpan={reasonCount} className="overview-cell">
          {confidenceLevel}
        </TableCell>
        <TableCell
          rowSpan={reasonCount}
          className="overview-cell"
          align="right"
        >
          {score}
        </TableCell>
        {reasonCount > 0 && <TableCell>{reasons[0]}</TableCell>}
      </TableRow>
      {reasons.slice(1).map((reason) => (
        <TableRow key={nanoid()}>
          <TableCell>{reason}</TableCell>
        </TableRow>
      ))}
    </>
  );
}
