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

import './culprit_verification_table.css';

import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import { GenericSuspect } from '@/bisection/types';

import { CulpritVerificationTableRow } from './culprit_verification_table_row';

function getRows(suspects: readonly GenericSuspect[]) {
  return suspects.map((suspect) => (
    <CulpritVerificationTableRow key={suspect.commit.id} suspect={suspect} />
  ));
}

export interface CulpritVerificationTableProps {
  readonly suspects: readonly GenericSuspect[];
}

export function CulpritVerificationTable({
  suspects,
}: CulpritVerificationTableProps) {
  if (suspects.length === 0) {
    return (
      <span className="data-placeholder">No culprit verification results</span>
    );
  }
  return (
    <TableContainer
      component={Paper}
      className="culprit-verification-table-container"
    >
      <Table className="culprit-verification-table" size="small">
        <TableHead>
          <TableRow>
            <TableCell>Suspect CL</TableCell>
            <TableCell>Type</TableCell>
            <TableCell>Verification Status</TableCell>
            <TableCell>Reruns</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>{getRows(suspects)}</TableBody>
      </Table>
    </TableContainer>
  );
}
