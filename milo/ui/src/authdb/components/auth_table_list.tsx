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

import '@/authdb/components/groups.css';

import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import { GroupLink } from '@/authdb/components/group_link';
import { AuthLookupLink } from '@/authdb/components/lookup_link';

interface TableListProps {
  items: string[];
  name: string;
  renderAsGroupLinks?: boolean;
}

export function AuthTableList({
  items,
  name,
  renderAsGroupLinks,
}: TableListProps) {
  return (
    <TableContainer data-testid="auth-table-list">
      <Table sx={{ p: 0, pt: '15px', width: '100%' }}>
        <TableBody>
          <TableRow>
            <TableCell colSpan={2} sx={{ pb: 0 }} style={{ minHeight: '45px' }}>
              <Typography variant="h6">{name}</Typography>
            </TableCell>
          </TableRow>
          {items.map((item) => (
            <TableRow
              key={item}
              style={{ height: '34px' }}
              sx={{ borderBottom: '1px solid rgb(224, 224, 224)' }}
              data-testid={`item-row-${item}`}
            >
              <TableCell sx={{ p: 0, pt: '1px' }} style={{ minHeight: '30px' }}>
                {renderAsGroupLinks ? (
                  <GroupLink name={item} />
                ) : (
                  <AuthLookupLink principal={item} />
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}
