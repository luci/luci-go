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
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { createTheme, ThemeProvider } from '@mui/material/styles';
import { useQuery } from '@tanstack/react-query';
import { DateTime } from 'luxon';

import { stripPrefix, stripPrefixFromItems } from '@/authdb/common/helpers';
import { useAuthServiceChangeLogsClient } from '@/authdb/hooks/prpc_clients';
import {
  ParamsPager,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { Timestamp } from '@/common/components/timestamp';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { AuthDBChange } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/changelogs.pb';

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
          padding: '5px',
        },
      },
    },
  },
});

const CHANGE_FORMATTERS = new Map([
  ['GROUP_MEMBERS_ADDED', formatMembers],
  ['GROUP_MEMBERS_REMOVED', formatMembers],
  [
    'GROUP_GLOBS_ADDED',
    (change: AuthDBChange) =>
      formatPrincipals(change.globs as string[], change.changeType),
  ],
  [
    'GROUP_GLOBS_REMOVED',
    (change: AuthDBChange) =>
      formatPrincipals(change.globs as string[], change.changeType),
  ],

  [
    'GROUP_NESTED_ADDED',
    (change: AuthDBChange) =>
      formatList(change.nested as string[], change.changeType),
  ],
  [
    'GROUP_NESTED_REMOVED',
    (change: AuthDBChange) =>
      formatList(change.nested as string[], change.changeType),
  ],

  [
    'GROUP_DESCRIPTION_CHANGED',
    (change: AuthDBChange) => formatValue(change.description),
  ],
  ['GROUP_CREATED', (change) => formatValue(change.description)],

  [
    'GROUP_OWNERS_CHANGED',
    (change: AuthDBChange) =>
      formatValue(`${change.oldOwners} \u2192 ${change.owners}`),
  ],
]);

function formatValue(value: string) {
  if (!value) {
    // No tooltip needed for an undefined/empty value.
    return null;
  }
  return <Typography variant="body2">{value}</Typography>;
}

function formatList(items: string[], changeType: string) {
  return (
    <>
      {items.map((item, index) => (
        <Typography variant="body2" key={index}>
          {getDetailFromChangeType(changeType, item)}
        </Typography>
      ))}
    </>
  );
}

function formatMembers(change: AuthDBChange) {
  if (change.numRedacted > 0) {
    const plural = change.numRedacted > 1 ? 's' : '';
    return formatList(
      [`${change.numRedacted} member${plural}`],
      change.changeType,
    );
  }

  return formatPrincipals(change.members as string[], change.changeType);
}

function formatPrincipals(principals: string[], changeType: string) {
  const names = stripPrefixFromItems('user', principals);
  return formatList(names, changeType);
}

const getDetailsFromChange = (change: AuthDBChange) => {
  if (CHANGE_FORMATTERS.has(change.changeType)) {
    const formatter = CHANGE_FORMATTERS.get(change.changeType)!;
    return formatter(change) || null;
  }
  return null;
};

const getDetailFromChangeType = (changeType: string, value: string) => {
  let changeColor;
  let changePrefix;
  if (changeType.includes('ADDED') || changeType.includes('CREATED')) {
    changeColor = 'green';
    changePrefix = '+';
  } else if (changeType.includes('REMOVED')) {
    changeColor = 'red';
    changePrefix = '-';
  } else {
    return null;
  }
  return (
    <Typography
      variant="body2"
      style={{ color: changeColor }}
      sx={{ pr: '5px' }}
    >
      {changePrefix} {value}
    </Typography>
  );
};

interface GroupHistoryProps {
  name: string;
}

export function GroupHistory({ name }: GroupHistoryProps) {
  const client = useAuthServiceChangeLogsClient();
  const pagerCtx = usePagerContext({
    pageSizeOptions: [5, 10, 15, 20],
    defaultPageSize: 10,
  });
  const [searchParams] = useSyncedSearchParams();
  const pageSize = getPageSize(pagerCtx, searchParams);
  const pageToken = getPageToken(pagerCtx, searchParams);

  const {
    isPending,
    isError,
    error,
    data: response,
  } = useQuery({
    ...client.ListChangeLogs.query({
      target: `AuthGroup$${name}`,
      pageSize: pageSize,
      authDbRev: '0',
      pageToken: pageToken,
    }),
    refetchOnWindowFocus: false,
  });

  if (isPending) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  if (isError) {
    return (
      <div className="section" data-testid="group-descendants-error">
        <Alert severity="error">
          <AlertTitle>Failed to load group descendants </AlertTitle>
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  return (
    <>
      <ThemeProvider theme={theme}>
        <TableContainer data-testid="history-table">
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Change</TableCell>
                <TableCell>When</TableCell>
                <TableCell>Modified By</TableCell>
                <TableCell>Details</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {response?.changes.map((item, index) => {
                return (
                  <TableRow key={index}>
                    <TableCell>
                      <Typography variant="body2">{item.changeType}</Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2">
                        <Timestamp
                          datetime={DateTime.fromISO(item.when || '')}
                        />
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2">
                        {stripPrefix('user', item.who)}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2">
                        {getDetailsFromChange(item)}
                      </Typography>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </TableContainer>
        <Box
          style={{
            display: 'flex',
            justifyContent: 'center',
          }}
        >
          <ParamsPager
            pagerCtx={pagerCtx}
            nextPageToken={response?.nextPageToken || ''}
          />
        </Box>
      </ThemeProvider>
    </>
  );
}
