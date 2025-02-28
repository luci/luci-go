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
import { useQuery } from '@tanstack/react-query';

import { stripPrefix } from '@/authdb/common/helpers';
import { useAuthServiceGroupsClient } from '@/authdb/hooks/prpc_clients';

import { GroupLink } from './group_link';

interface GroupListingProps {
  name: string;
}

export function GroupListing({ name }: GroupListingProps) {
  const client = useAuthServiceGroupsClient();

  const {
    isLoading,
    isError,
    error,
    data: response,
  } = useQuery({
    ...client.GetExpandedGroup.query({ name: name }),
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
      <div className="section" data-testid="group-listing-error">
        <Alert severity="error">
          <AlertTitle>Failed to load group listing </AlertTitle>
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  const members =
    response?.members?.map((member) => stripPrefix('user', member)) ||
    ([] as string[]);
  const globs = response?.globs as string[];
  const nested = response?.nested as string[];

  return (
    <>
      <TableContainer sx={{ p: 0 }}>
        <Table data-testid="listing-table">
          <TableBody>
            <TableRow>
              <TableCell>
                <Typography variant="h6">Members</Typography>
              </TableCell>
            </TableRow>
            <TableRow>
              <TableCell sx={{ pb: '16px' }}>
                {members.length > 0 ? (
                  <ul>
                    {members?.map((member) => {
                      return (
                        <li key={member}>
                          <Typography variant="body2">{member}</Typography>
                        </li>
                      );
                    })}
                  </ul>
                ) : (
                  <Typography
                    variant="body2"
                    sx={{ fontStyle: 'italic', pl: '20px', color: 'grey' }}
                  >
                    No members
                  </Typography>
                )}
              </TableCell>
            </TableRow>
            <TableRow>
              <TableCell>
                <Typography variant="h6">Globs</Typography>
              </TableCell>
            </TableRow>
            <TableRow>
              <TableCell sx={{ pb: '16px' }}>
                {globs.length > 0 ? (
                  <ul>
                    {globs?.map((glob) => {
                      return (
                        <li key={glob}>
                          <Typography variant="body2">{glob}</Typography>
                        </li>
                      );
                    })}
                  </ul>
                ) : (
                  <Typography
                    variant="body2"
                    sx={{ fontStyle: 'italic', pl: '20px', color: 'grey' }}
                  >
                    No globs
                  </Typography>
                )}
              </TableCell>
            </TableRow>
            <TableRow>
              <TableCell>
                <Typography variant="h6">Nested Groups</Typography>
              </TableCell>
            </TableRow>
            <TableRow>
              <TableCell sx={{ pb: '16px' }}>
                {nested.length > 0 ? (
                  <ul>
                    {nested?.map((group) => {
                      return (
                        <li key={group}>
                          <GroupLink name={group}></GroupLink>
                        </li>
                      );
                    })}
                  </ul>
                ) : (
                  <Typography
                    variant="body2"
                    sx={{ fontStyle: 'italic', pl: '20px', color: 'grey' }}
                  >
                    No nested groups
                  </Typography>
                )}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </TableContainer>
    </>
  );
}
