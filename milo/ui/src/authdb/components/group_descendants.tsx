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
import TableContainer from '@mui/material/TableContainer';
import { useQuery } from '@tanstack/react-query';

import { stripPrefix } from '@/authdb/common/helpers';
import { useAuthServiceGroupsClient } from '@/authdb/hooks/prpc_clients';

import { CollapsibleList } from './collapsible_list';

interface GroupDescendantsProps {
  name: string;
}

export function GroupDescendants({ name }: GroupDescendantsProps) {
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
      <div className="section" data-testid="group-descendants-error">
        <Alert severity="error">
          <AlertTitle>Failed to load group descendants </AlertTitle>
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
        <Table data-testid="descendants-table">
          <TableBody>
            <CollapsibleList
              items={members}
              renderAsGroupLinks={false}
              title="Members"
            />
            <CollapsibleList
              items={globs}
              renderAsGroupLinks={false}
              title="Globs"
            />
            <CollapsibleList
              items={nested}
              renderAsGroupLinks={true}
              title="Nested Groups"
            />
          </TableBody>
        </Table>
      </TableContainer>
    </>
  );
}
