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

import './culprits_table.css';

import { nanoid } from '@reduxjs/toolkit';

import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import { NoDataMessageRow } from '../no_data_message_row/no_data_message_row';
import {
  Culprit,
  CulpritAction,
  CulpritActionType,
} from '../../services/luci_bisection';
import { getCommitShortHash } from '../../tools/commit_formatters';

interface CulpritsTableProps {
  culprits: Culprit[];
}

interface CulpritTableRowProps {
  culprit: Culprit;
}

interface CulpritActionTableCellProps {
  action: CulpritAction | null | undefined;
}

const CULPRIT_ACTION_DESCRIPTIONS: Record<CulpritActionType, string> = {
  CULPRIT_ACTION_TYPE_UNSPECIFIED: '',
  CULPRIT_AUTO_REVERTED: 'This culprit has been auto-reverted',
  REVERT_CL_CREATED: 'A revert CL has been created for this culprit',
  CULPRIT_CL_COMMENTED:
    'A comment was added on the original code review for this culprit',
  BUG_COMMENTED: 'A comment was added on a related bug',
};

const CulpritActionTableCell = ({ action }: CulpritActionTableCellProps) => {
  if (action == null) {
    return <TableCell></TableCell>;
  }

  let linkText = '';
  let url = '';
  switch (action.actionType) {
    case 'CULPRIT_AUTO_REVERTED':
    case 'REVERT_CL_CREATED':
      linkText = 'revert CL';
      url = action.revertClUrl || '';
      break;
    case 'BUG_COMMENTED':
      linkText = 'bug';
      url = action.bugUrl || '';
      break;
    default:
    // continue
  }

  return (
    <TableCell>
      {`${CULPRIT_ACTION_DESCRIPTIONS[action.actionType]}${
        linkText && url ? ': ' : ''
      }`}
      {linkText && url && (
        <Link href={url} target='_blank' rel='noreferrer' underline='always'>
          {linkText}
        </Link>
      )}
    </TableCell>
  );
};

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
            target='_blank'
            rel='noreferrer'
            underline='always'
          >
            {culpritDescription}
          </Link>
        </TableCell>
        {culpritAction.length > 0 ? (
          <CulpritActionTableCell action={culpritAction[0]} />
        ) : (
          <TableCell className='data-placeholder'>
            No actions by LUCI Bisection for this culprit
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
  return (
    <TableContainer className='culprits-table' component={Paper}>
      <Table size='small'>
        <TableHead>
          <TableRow>
            <TableCell>Culprit CL</TableCell>
            <TableCell>Actions</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {culprits.length === 0 ? (
            <NoDataMessageRow message='No culprit found' columns={2} />
          ) : (
            culprits.map((culprit) => (
              <CulpritTableRow key={culprit.commit.id} culprit={culprit} />
            ))
          )}
        </TableBody>
      </Table>
    </TableContainer>
  );
};
