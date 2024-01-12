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

import './culprits_table.css';

import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import { nanoid } from 'nanoid';

import { PlainTable } from '@/bisection/components/plain_table';
import { getCommitShortHash } from '@/bisection/tools/commit_formatters';
import { displayRerunStatus } from '@/bisection/tools/info_display';
import { linkToBuild } from '@/bisection/tools/link_constructors';
import {
  Culprit,
  CulpritAction,
  CulpritActionType,
  CulpritInactionReason,
  SuspectVerificationDetails,
} from '@/common/services/luci_bisection';

interface CulpritsTableProps {
  culprits?: Culprit[];
}

interface CulpritTableRowProps {
  culprit: Culprit;
}

interface CulpritActionTableCellProps {
  action: CulpritAction | null | undefined;
}

interface VerificationDetailsTableProps {
  details: SuspectVerificationDetails;
}

const CULPRIT_ACTION_DESCRIPTIONS: Record<CulpritActionType, string> = {
  CULPRIT_ACTION_TYPE_UNSPECIFIED: '',
  NO_ACTION:
    'No actions have been performed by LUCI Bisection for this culprit',
  CULPRIT_AUTO_REVERTED: 'This culprit has been auto-reverted',
  REVERT_CL_CREATED: 'A revert CL has been created for this culprit',
  CULPRIT_CL_COMMENTED:
    'A comment was added on the original code review for this culprit',
  BUG_COMMENTED: 'A comment was added on a related bug',
  EXISTING_REVERT_CL_COMMENTED:
    'A comment was added to the code review for an existing revert of this culprit',
};

const CULPRIT_INACTION_EXPLANATIONS: Record<CulpritInactionReason, string> = {
  CULPRIT_INACTION_REASON_UNSPECIFIED: '',
  REVERTED_BY_BISECTION:
    'it has been reverted as the culprit of another LUCI Bisection analysis',
  REVERTED_MANUALLY: 'it has already been reverted',
  REVERT_OWNED_BY_BISECTION:
    'the revert was created by another LUCI Bisection analysis',
  REVERT_HAS_COMMENT:
    'the revert already has a comment from another LUCI Bisection analysis',
  CULPRIT_HAS_COMMENT:
    'the culprit already has a comment from another LUCI Bisection analysis',
  ANALYSIS_CANCELED: 'the analysis was canceled',
  ACTIONS_DISABLED: 'actions on culprits are disabled',
  TEST_NO_LONGER_UNEXPECTED: 'the test is no longer deterministically failing',
};

const INACTION_REASONS_WITH_REVERT_LINK: CulpritInactionReason[] = [
  'REVERTED_BY_BISECTION',
  'REVERTED_MANUALLY',
  'REVERT_OWNED_BY_BISECTION',
  'REVERT_HAS_COMMENT',
];

function CulpritActionTableCell({ action }: CulpritActionTableCellProps) {
  if (action == null) {
    return <TableCell></TableCell>;
  }

  let linkText = '';
  let url = '';
  switch (action.actionType) {
    case 'CULPRIT_AUTO_REVERTED':
    case 'REVERT_CL_CREATED':
    case 'EXISTING_REVERT_CL_COMMENTED':
      linkText = 'revert CL';
      url = action.revertClUrl || '';
      break;
    case 'BUG_COMMENTED':
      linkText = 'bug';
      url = action.bugUrl || '';
      break;
    case 'NO_ACTION': {
      const reason: CulpritInactionReason =
        action.inactionReason || 'CULPRIT_INACTION_REASON_UNSPECIFIED';
      if (INACTION_REASONS_WITH_REVERT_LINK.includes(reason)) {
        linkText = 'revert CL';
        url = action.revertClUrl || '';
      }
      break;
    }
    default:
    // continue
  }

  const description = CULPRIT_ACTION_DESCRIPTIONS[action.actionType];
  let inactionExplanation = '';
  if (action.actionType === 'NO_ACTION' && action.inactionReason) {
    inactionExplanation =
      ' because ' + CULPRIT_INACTION_EXPLANATIONS[action.inactionReason];
  }

  return (
    <TableCell>
      {`${description}${inactionExplanation}${linkText && url ? ': ' : ''}`}
      {linkText && url && (
        <Link href={url} target="_blank" rel="noreferrer" underline="always">
          {linkText}
        </Link>
      )}
      .
    </TableCell>
  );
}

export function VerificationDetailsTable({
  details,
}: VerificationDetailsTableProps) {
  if (!details.suspectRerun && !details.parentRerun) {
    return <>No rerun found</>;
  }
  return (
    <PlainTable>
      <TableBody>
        <TableRow>
          <TableCell>
            Culprit commit:{' '}
            {details.suspectRerun && (
              <Link
                href={linkToBuild(details.suspectRerun.bbid).url}
                target="_blank"
                rel="noreferrer"
                underline="always"
              >
                {displayRerunStatus(
                  details.suspectRerun.rerunResult?.rerunStatus,
                )}
              </Link>
            )}
          </TableCell>
        </TableRow>
        <TableRow>
          <TableCell>
            Parent commit:{' '}
            {details.parentRerun && (
              <Link
                href={linkToBuild(details.parentRerun.bbid).url}
                target="_blank"
                rel="noreferrer"
                underline="always"
              >
                {displayRerunStatus(
                  details.parentRerun.rerunResult?.rerunStatus,
                )}
              </Link>
            )}
          </TableCell>
        </TableRow>
      </TableBody>
    </PlainTable>
  );
}

const CulpritTableRow = ({ culprit }: CulpritTableRowProps) => {
  const { commit, reviewUrl, reviewTitle } = culprit;
  let culpritDescription = getCommitShortHash(commit.id);
  if (reviewTitle) {
    culpritDescription += `: ${reviewTitle}`;
  }

  const culpritAction = culprit.culpritAction || [];
  let rowSpan = 1;
  if (culpritAction.length > rowSpan) {
    rowSpan = culpritAction.length;
  }

  return (
    <>
      <TableRow>
        <TableCell rowSpan={rowSpan}>
          <Link
            href={reviewUrl}
            target="_blank"
            rel="noreferrer"
            underline="always"
          >
            {culpritDescription}
          </Link>
        </TableCell>
        <TableCell>
          <VerificationDetailsTable details={culprit.verificationDetails} />
        </TableCell>
        {culpritAction.length > 0 ? (
          <CulpritActionTableCell action={culpritAction[0]} />
        ) : (
          <TableCell>
            <span className="data-placeholder">
              No actions by LUCI Bisection for this culprit
            </span>
          </TableCell>
        )}
      </TableRow>
      {culpritAction.slice(1).map((action) => (
        <TableRow key={nanoid()}>
          <CulpritActionTableCell action={action} />
        </TableRow>
      ))}
    </>
  );
};

export const CulpritsTable = ({ culprits }: CulpritsTableProps) => {
  if (!culprits || culprits.length == 0) {
    return (
      <span className="data-placeholder" data-testid="culprits-table">
        No culprit found
      </span>
    );
  }

  return (
    <TableContainer
      className="culprits-table"
      component={Paper}
      data-testid="culprits-table"
    >
      <Table size="small">
        <colgroup>
          <col style={{ width: '35%' }} />
          <col style={{ width: '20%' }} />
          <col style={{ width: '45%' }} />
        </colgroup>
        <TableHead>
          <TableRow>
            <TableCell>Culprit CL</TableCell>
            <TableCell>Verification Details</TableCell>
            <TableCell>Actions</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {culprits.map((culprit) => (
            <CulpritTableRow key={culprit.commit.id} culprit={culprit} />
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};
