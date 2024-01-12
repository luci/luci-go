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

import './culprit_verification_table_row.css';

import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';

import { VerificationDetailsTable } from '@/bisection/components/culprits_table';
import { getCommitShortHash } from '@/bisection/tools/commit_formatters';
import { Suspect } from '@/common/services/luci_bisection';

interface Props {
  suspect: Suspect;
}

export function CulpritVerificationTableRow({ suspect }: Props) {
  const { commit, reviewUrl, reviewTitle, verificationDetails, type } = suspect;

  let suspectDescription = getCommitShortHash(commit.id);
  if (reviewTitle) {
    suspectDescription += `: ${reviewTitle}`;
  }
  return (
    <>
      <TableRow data-testid="culprit_verification_table_row">
        <TableCell className="overview-cell">
          <Link
            href={reviewUrl}
            target="_blank"
            rel="noreferrer"
            underline="always"
          >
            {suspectDescription}
          </Link>
        </TableCell>
        <TableCell className="overview-cell">{type}</TableCell>
        <TableCell className="overview-cell">
          {verificationDetails.status}
        </TableCell>
        <TableCell className="overview-cell">
          <VerificationDetailsTable details={verificationDetails} />
        </TableCell>
      </TableRow>
    </>
  );
}
