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

import { stripPrefix, sortGroupsByName } from '@/authdb/common/helpers';
import { useAuthServiceGroupsClient } from '@/authdb/hooks/prpc_clients';
import {
  PrincipalKind,
  Subgraph,
  Node,
  AuthGroup,
} from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

import { CollapsibleList } from './collapsible_list';

import './groups.css';

// For each long path in the list, kick out the middle and replace it with ''.
const shortenInclusionPaths = (paths: string[]) => {
  const out: string[] = [];
  const seen = new Set();

  paths.forEach((path) => {
    if (path.length <= 3) {
      out.push(path); // short enough already
      return;
    }
    const shorter = [path[0], '', path[path.length - 1]];
    const key = shorter.join('\n');
    if (!seen.has(key)) {
      seen.add(key);
      out.push(...shorter);
    }
  });

  return out;
};

export const interpretLookupResults = (subgraph: Subgraph) => {
  // Note: the principal is always represented by nodes[0] per API guarantee.
  const nodes = subgraph.nodes;
  const principalNode = nodes[0];
  const principal = principalNode.principal;

  const includers = new Map();
  const getIncluder = (groupName: string) => {
    if (!includers.has(groupName)) {
      includers.set(groupName, {
        name: groupName,
        includesDirectly: false,
        includesViaGlobs: [],
        includesIndirectly: [],
      });
    }
    return includers.get(groupName);
  };

  const enumeratePaths = (
    current: Node[],
    visitCallback: (path: Node[]) => void,
  ) => {
    visitCallback(current);
    const lastNode = current[current.length - 1];
    if (lastNode.includedBy) {
      lastNode.includedBy.forEach((idx) => {
        const node = nodes[idx];
        current.push(node);
        enumeratePaths(current, visitCallback);
        current.pop();
      });
    }
  };

  const visitor = (path: Node[]) => {
    if (path.length === 1) {
      return; // the trivial [principal] path
    }

    const lastNode = path[path.length - 1];
    if (lastNode!.principal!.kind !== PrincipalKind.GROUP) {
      return; // we are only interested in examining groups; skip GLOBs
    }

    const groupIncluder = getIncluder(lastNode!.principal!.name);
    if (path.length === 2) {
      // The entire path is 'principalNode -> lastNode', meaning group
      // 'last' includes the principal directly.
      groupIncluder.includesDirectly = true;
    } else if (
      path.length === 3 &&
      path[1]!.principal!.kind === PrincipalKind.GLOB
    ) {
      // The entire path is 'principalNode -> GLOB -> lastNode', meaning
      // 'last' includes the principal via the GLOB.
      groupIncluder.includesViaGlobs.push(
        stripPrefix('user', path[1]!.principal!.name),
      );
    } else {
      // Some arbitrarily long indirect inclusion path. Just record all
      // group names in it (skipping GLOBs). Skip the root principal
      // itself (path[0]) and the currrently analyzed node (path[-1]);
      // it's not useful information as it's the same for all paths.
      const groupNames = [];
      for (let i = 1; i < path.length - 1; i++) {
        if (path[i]!.principal!.kind === PrincipalKind.GROUP) {
          groupNames.push(path[i]!.principal!.name);
        }
      }
      groupIncluder.includesIndirectly.push(groupNames);
    }
  };
  enumeratePaths([principalNode], visitor);

  // Finally, massage the findings for easier display. Note that
  // directIncluders and indirectIncluders are NOT disjoint sets.
  let directIncluders: AuthGroup[] = [];
  let indirectIncluders: AuthGroup[] = [];
  includers.forEach((inc) => {
    if (inc.includesDirectly || inc.includesViaGlobs.length > 0) {
      directIncluders.push(inc);
    }

    if (inc.includesIndirectly.length > 0) {
      // Long inclusion paths look like data dumps in UI and don't fit
      // most of the time. The most interesting components are at the
      // ends, so keep only them.
      inc.includesIndirectly = shortenInclusionPaths(inc.includesIndirectly);
      indirectIncluders.push(inc);
    }
  });

  directIncluders = sortGroupsByName(directIncluders);
  indirectIncluders = sortGroupsByName(indirectIncluders);

  return {
    principalName: stripPrefix('user', principal!.name),
    principalIsGroup: principal!.kind === PrincipalKind.GROUP,
    includers: includers, // will be used to construct popovers
    directIncluders: directIncluders,
    indirectIncluders: indirectIncluders,
  };
};

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

  const getGroupNames = (groupsList: AuthGroup[]) => {
    return groupsList.map((group) => {
      return group.name;
    });
  };

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
