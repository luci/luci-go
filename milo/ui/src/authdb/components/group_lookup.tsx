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

import { getGroupNames, interpretLookupResults } from '@/authdb/common/helpers';
import { useAuthServiceGroupsClient } from '@/authdb/hooks/prpc_clients';
import { PrincipalKind } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

import { CollapsibleList } from './collapsible_list';

import './groups.css';

interface GroupLookupProps {
  name: string;
}

export function GroupLookup({ name }: GroupLookupProps) {
  const client = useAuthServiceGroupsClient();
  const principalReq = {
    name: name,
    kind: PrincipalKind.GROUP,
  };
  const {
    isLoading,
    isError,
    error,
    data: response,
  } = useQuery({
    ...client.GetSubgraph.query({ principal: principalReq }),
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
      <div className="section" data-testid="group-lookup-error">
        <Alert severity="error">
          <AlertTitle>Failed to load group ancestors </AlertTitle>
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
      <TableContainer sx={{ p: 0 }}>
        <Table data-testid="lookup-table">
          <TableBody>
            <CollapsibleList
              items={getGroupNames(directIncluders)}
              renderAsGroupLinks={true}
              title="Directly included by"
            />
            <CollapsibleList
              items={getGroupNames(indirectIncluders)}
              renderAsGroupLinks={true}
              title="Indirectly included by"
            />
          </TableBody>
        </Table>
      </TableContainer>
    </>
  );
}
