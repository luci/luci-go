// Copyright 2024 The LUCI Authors.
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

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
} from '@mui/material';

import { Status } from '@/proto/go.chromium.org/luci/tree_status/proto/v1/tree_status.pb';

import { TreeStatusRow } from './tree_status_row';

interface TreeStatusTableProps {
  status: readonly Status[];
}

// An TreeStatusTable shows the most recent tree status updates.
export function TreeStatusTable({ status }: TreeStatusTableProps) {
  if (!status.length) {
    return null;
  }

  const fit = { width: '1px', whiteSpace: 'nowrap' };
  return (
    <Table size="small">
      <TableHead>
        <TableRow>
          <TableCell sx={fit}>Tree Status</TableCell>
          <TableCell>Message</TableCell>
          <TableCell sx={fit}>When</TableCell>
          <TableCell sx={fit}>Who</TableCell>
        </TableRow>
      </TableHead>
      <TableBody>
        {status.map((status) => {
          return <TreeStatusRow key={status.name} status={status} />;
        })}
      </TableBody>
    </Table>
  );
}
