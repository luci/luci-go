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

import ParkIcon from '@mui/icons-material/Park';
import {
  Chip,
  Link,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { LinkifiedText } from '@/common/components/linkified_text';
import {
  statusColor,
  statusText,
} from '@/common/tools/tree_status/tree_status_utils';
import { TreeJson } from '@/monitoringv2/util/server_json';
import {
  GeneralState,
  ListStatusRequest,
  Status,
} from '@/proto/go.chromium.org/luci/tree_status/proto/v1/tree_status.pb';
import { useTreeStatusClient } from '@/tree_status/hooks/prpc_clients';

interface TreeStatusSideNavProps {
  tree: TreeJson;
}

/**
 * Displays a summary of the tree status suitable for a side navigation bar.
 */
export const TreeStatusSideNav = ({ tree }: TreeStatusSideNavProps) => {
  const treeStatusClient = useTreeStatusClient();
  const statusQuery = useQuery({
    ...treeStatusClient.ListStatus.query(
      ListStatusRequest.fromPartial({
        parent: `trees/${tree.treeStatusName}/status`,
        pageSize: 1, // We only need the latest status for the summary.
      }),
    ),
    refetchInterval: 60000,
    enabled: !!tree.treeStatusName,
  });

  if (statusQuery.isError) {
    throw statusQuery.error;
  }

  if (statusQuery.isPending) {
    return (
      <ListItem disablePadding>
        <ListItemButton>
          <ListItemIcon>
            <ParkIcon />
          </ListItemIcon>
          <ListItemText primary="Tree Status" />
          <Chip label="Loading..." variant="outlined" />
        </ListItemButton>
      </ListItem>
    );
  }

  let status = statusQuery.data?.status;
  if (status?.length === 0) {
    status = [
      Status.fromPartial({
        message:
          'Default open status as there are no updates in the last 140 days',
        generalState: GeneralState.OPEN,
      }),
    ];
  }

  const latest = status?.[0];
  const color = statusColor(latest?.generalState);
  const text = statusText(latest?.generalState);

  return (
    <ListItem disablePadding>
      <ListItemButton
        component={Link}
        href={`/ui/labs/tree-status/${tree.treeStatusName}`}
        target="_blank"
      >
        <ListItemIcon>
          <ParkIcon sx={{ color }} />
        </ListItemIcon>
        <ListItemText
          primary="Tree Status"
          sx={{ color }}
          secondary={<LinkifiedText text={latest.message} />}
          title={latest.message}
          secondaryTypographyProps={{
            sx: {
              fontSize: '0.75rem',
              display: '-webkit-box',
              WebkitLineClamp: 2,
              WebkitBoxOrient: 'vertical',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              maxHeight: '2.5em',
              opacity: '70%',
              color,
            },
          }}
        />
        <Chip
          label={text}
          variant="outlined"
          sx={{
            color,
            borderColor: color,
            marginLeft: '8px',
          }}
        />
      </ListItemButton>
    </ListItem>
  );
};
