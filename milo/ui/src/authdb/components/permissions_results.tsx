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
import './groups.css';

import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Grid from '@mui/material/Grid2';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableContainer from '@mui/material/TableContainer';
import Typography from '@mui/material/Typography';
import { useQuery } from '@tanstack/react-query';
import { Fragment } from 'react';

import { principalAsRequestProto } from '@/authdb/common/helpers';
import { useAuthServiceAuthDBClient } from '@/authdb/hooks/prpc_clients';
import { RealmPermissions } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/authdb.pb';

import { CollapsibleList } from './collapsible_list';
interface PermissionsResultsProps {
  principal: string;
}

export function PermissionsResults({ principal }: PermissionsResultsProps) {
  const client = useAuthServiceAuthDBClient();
  const request = principalAsRequestProto(principal);
  const {
    fetchStatus,
    isLoading,
    isError,
    error,
    data: response,
  } = useQuery({
    ...client.GetPrincipalPermissions.query(request),
    refetchOnWindowFocus: false,
    enabled: principal !== '',
  });

  // Leave results empty if query is empty.
  if (fetchStatus === 'idle' && isLoading) {
    return <></>;
  }

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  if (isError) {
    return (
      <div className="section" data-testid="group-permissions-error">
        <Alert severity="error">
          <AlertTitle>Failed to load permissions </AlertTitle>
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  const realmPermissions =
    (response.realmPermissions as RealmPermissions[]) || [];

  return (
    <>
      <TableContainer sx={{ p: 0 }}>
        <Table data-testid="permissions-table">
          <TableBody>
            {realmPermissions.length === 0 ? (
              <Typography variant="body2">No permissions found.</Typography>
            ) : (
              <>
                <Grid container>
                  <Grid size={6}>
                    {realmPermissions
                      .slice(0, realmPermissions.length / 2)
                      ?.map((realm) => {
                        return (
                          <Fragment key={realm.name}>
                            <CollapsibleList
                              items={realm.permissions as string[]}
                              renderAsGroupLinks={false}
                              title={realm.name}
                            />
                          </Fragment>
                        );
                      })}
                  </Grid>
                  <Grid size={6}>
                    {realmPermissions
                      .slice(
                        realmPermissions.length / 2,
                        realmPermissions.length,
                      )
                      ?.map((realm) => {
                        return (
                          <Fragment key={realm.name}>
                            <CollapsibleList
                              items={realm.permissions as string[]}
                              renderAsGroupLinks={false}
                              title={realm.name}
                              key={realm.name}
                            />
                          </Fragment>
                        );
                      })}
                  </Grid>
                </Grid>
              </>
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </>
  );
}
