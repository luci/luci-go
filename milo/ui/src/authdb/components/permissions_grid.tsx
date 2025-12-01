// Copyright 2025 The LUCI Authors.
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

import Chip from '@mui/material/Chip';
import Grid from '@mui/material/Grid2';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableContainer from '@mui/material/TableContainer';
import Typography from '@mui/material/Typography';
import { useState } from 'react';

import { CollapsibleList } from '@/authdb/components/collapsible_list';
import { RealmPermissions } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/authdb.pb';

interface PermissionsGridProps {
  permissions: RealmPermissions[];
  fullWidth?: boolean;
}

export function PermissionsGrid({
  permissions,
  fullWidth = false,
}: PermissionsGridProps) {
  const [allExpanded, setAllExpanded] = useState(true);

  const toggleExpansion = () => {
    setAllExpanded(!allExpanded);
  };

  const columnSizes = fullWidth ? 12 : { xs: 12, lg: 6, xxl: 4 };

  return (
    <Grid container spacing={1} data-testid="permissions-grid">
      {permissions.length === 0 ? (
        <Grid size={12}>
          <Typography
            component="div"
            variant="body2"
            color="textSecondary"
            sx={{
              fontStyle: 'italic',
            }}
          >
            No permissions found.
          </Typography>
        </Grid>
      ) : (
        <>
          <Grid size={12}>
            <Chip
              variant="outlined"
              size="small"
              onClick={toggleExpansion}
              label={allExpanded ? 'Collapse all' : 'Expand all'}
            ></Chip>
          </Grid>
          {permissions.map((realm) => {
            return (
              <Grid key={realm.name} size={columnSizes}>
                <TableContainer sx={{ p: 0 }}>
                  <Table>
                    <TableBody>
                      <CollapsibleList
                        title={realm.name}
                        items={realm.permissions as string[]}
                        globallyExpanded={allExpanded}
                      />
                    </TableBody>
                  </Table>
                </TableContainer>
              </Grid>
            );
          })}
        </>
      )}
    </Grid>
  );
}
