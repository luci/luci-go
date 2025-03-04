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
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { createTheme, ThemeProvider } from '@mui/material/styles';
import { useQuery } from '@tanstack/react-query';

import { useAuthServiceGroupsClient } from '@/authdb/hooks/prpc_clients';
import { PrincipalKind } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

import { GroupLink } from './group_link';
import { interpretLookupResults } from './group_lookup';

interface LookupResultsProps {
  name: string;
}

const theme = createTheme({
  typography: {
    h6: {
      color: 'black',
      margin: '0',
      padding: '0',
      fontSize: '1.17em',
      fontWeight: 'bold',
    },
  },
  components: {
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderBottom: 'none',
          paddingLeft: '0',
          paddingBottom: '5px',
        },
      },
    },
  },
});

export function LookupResults({ name }: LookupResultsProps) {
  const client = useAuthServiceGroupsClient();

  // Determine principal type of search query.
  const isEmail = name.indexOf('@') !== -1 && name.indexOf('/') === -1;
  const isGlob = name.indexOf('*') !== -1;
  if ((isEmail || isGlob) && name.indexOf(':') === -1) {
    name = 'user:' + name;
  }
  let kind = PrincipalKind.GROUP;
  if (isGlob) {
    kind = PrincipalKind.GLOB;
  } else if (isEmail) {
    kind = PrincipalKind.IDENTITY;
  }
  const principalReq = {
    name: name,
    kind: kind,
  };

  const {
    fetchStatus,
    isLoading,
    isError,
    error,
    data: response,
  } = useQuery({
    ...client.GetSubgraph.query({ principal: principalReq }),
    refetchOnWindowFocus: false,
    enabled: name !== '',
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
          <AlertTitle>Failed to load group permissions </AlertTitle>
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  const summary = interpretLookupResults(response);
  const directIncluders = summary.directIncluders;
  const indirectIncluders = summary.indirectIncluders;

  return (
    <>
      <ThemeProvider theme={theme}>
        <TableContainer sx={{ p: 0 }}>
          <Table data-testid="lookup-table">
            <TableBody>
              <TableRow>
                <TableCell>
                  <Typography variant="h6">Directly included by</Typography>
                </TableCell>
                <TableCell>
                  <Typography variant="h6">Indirectly included by</Typography>
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell sx={{ pt: '8px', verticalAlign: 'top' }}>
                  {directIncluders.length > 0 ? (
                    <ul>
                      {directIncluders.map((group) => {
                        return (
                          <li key={group.name}>
                            <GroupLink name={group.name} />
                          </li>
                        );
                      })}
                    </ul>
                  ) : (
                    <Typography
                      variant="body2"
                      sx={{ fontStyle: 'italic', pl: '20px', color: 'grey' }}
                    >
                      None
                    </Typography>
                  )}
                </TableCell>
                <TableCell sx={{ pt: '8px', verticalAlign: 'top' }}>
                  {indirectIncluders.length > 0 ? (
                    <ul>
                      {indirectIncluders.map((group) => {
                        return (
                          <li key={group.name}>
                            <GroupLink name={group.name} />
                          </li>
                        );
                      })}
                    </ul>
                  ) : (
                    <Typography
                      variant="body2"
                      sx={{ fontStyle: 'italic', pl: '20px', color: 'grey' }}
                    >
                      None
                    </Typography>
                  )}
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </TableContainer>
      </ThemeProvider>
    </>
  );
}
