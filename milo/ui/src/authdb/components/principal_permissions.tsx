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

import '@/authdb/components/groups.css';

import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import { useQuery } from '@tanstack/react-query';

import { principalAsRequestProto } from '@/authdb/common/helpers';
import { PermissionsGrid } from '@/authdb/components/permissions_grid';
import { useAuthServiceAuthDBClient } from '@/authdb/hooks/prpc_clients';
import { RealmPermissions } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/authdb.pb';

interface PrincipalPermissionsProps {
  principal: string;
}

export function PrincipalPermissions({ principal }: PrincipalPermissionsProps) {
  const client = useAuthServiceAuthDBClient();
  const request = principalAsRequestProto(principal);
  const {
    fetchStatus,
    isPending,
    isError,
    error,
    data: response,
  } = useQuery({
    ...client.GetPrincipalPermissions.query(request),
    refetchOnWindowFocus: false,
    enabled: principal !== '',
  });

  // Leave results empty if query is empty.
  if (fetchStatus === 'idle' && isPending) {
    return <></>;
  }

  if (isPending) {
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

  return <PermissionsGrid permissions={realmPermissions} />;
}
