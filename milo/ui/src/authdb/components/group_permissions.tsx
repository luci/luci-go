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
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Typography from '@mui/material/Typography';
import { useQuery } from '@tanstack/react-query';

import { useAuthServiceAuthDBClient } from '@/authdb/hooks/prpc_clients';
import { RealmPermissions } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/authdb.pb';
import { PrincipalKind } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

interface GroupPermissionsProps {
  name: string;
}

export function GroupPermissions({ name }: GroupPermissionsProps) {
  const client = useAuthServiceAuthDBClient();
  const principal = {
    name: name,
    kind: PrincipalKind.GROUP,
  };
  const request = {
    principal: principal,
  };
  const {
    isLoading,
    isError,
    error,
    data: response,
  } = useQuery({
    ...client.GetPrincipalPermissions.query(request),
    refetchOnWindowFocus: false,
  });

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
          <AlertTitle>Failed to load group permissions </AlertTitle>
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  const realmPermissions =
    (response.realmPermissions as RealmPermissions[]) || [];

  return (
    <Box data-testid="group-permissions">
      {realmPermissions.length === 0 ? (
        <Typography variant="body2">No permissions found.</Typography>
      ) : (
        <>
          {realmPermissions?.map((realm) => {
            return (
              <>
                <Typography variant="h6">{realm.name}</Typography>
                <ul
                  style={{
                    margin: 0,
                    listStyleType: 'none',
                    paddingLeft: '20px',
                  }}
                >
                  {realm.permissions.map((permission) => {
                    return (
                      <li key={permission}>
                        <Typography variant="body2">{permission}</Typography>
                      </li>
                    );
                  })}
                </ul>
              </>
            );
          })}
        </>
      )}
    </Box>
  );
}
