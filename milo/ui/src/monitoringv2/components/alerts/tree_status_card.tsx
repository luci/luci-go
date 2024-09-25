// Copyright 2024 The LUCI Authors.
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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Button,
  Chip,
  Typography,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';

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
import { TreeStatusTable } from '@/tree_status/components/tree_status_table';
import { useTreeStatusClient } from '@/tree_status/hooks/prpc_clients';

interface AlertGroupProps {
  tree: TreeJson;
}
/**
 * A collapsible group of alerts like 'consistent failures' or 'new failures'.
 * Similar to BugGroup, but is never associated with a bug.
 */
export const TreeStatusCard = ({ tree }: AlertGroupProps) => {
  const treeStatusClient = useTreeStatusClient();
  const statusQuery = useQuery({
    // eslint-disable-next-line new-cap
    ...treeStatusClient.ListStatus.query(
      ListStatusRequest.fromPartial({
        parent: `trees/${tree.treeStatusName}/status`,
        pageSize: 5,
      }),
    ),
    refetchInterval: 60000,
    enabled: !!tree.treeStatusName,
  });

  if (statusQuery.isError) {
    throw statusQuery.error;
  }
  let status = statusQuery.data?.status;
  // If the server did not return any status, default to open.
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
  return (
    <Accordion>
      <AccordionSummary
        expandIcon={<ExpandMoreIcon />}
        sx={{ '.MuiAccordionSummary-content': { alignItems: 'baseline' } }}
      >
        <Chip
          sx={{
            marginRight: '8px',
            color: statusColor(latest?.generalState),
            borderColor: statusColor(latest?.generalState),
          }}
          label={
            statusQuery.isLoading ? 'Loading' : statusText(latest?.generalState)
          }
          variant="outlined"
        />
        <Typography>
          {tree.treeStatusName} Tree Status{' '}
          <small css={{ opacity: '50%' }}>
            <LinkifiedText text={latest?.message} />
          </small>
        </Typography>
      </AccordionSummary>
      <AccordionDetails>
        {status ? <TreeStatusTable status={status} /> : null}
        <Button
          sx={{ marginTop: '16px' }}
          size="small"
          component={Link}
          to={`/ui/tree-status/${tree.treeStatusName}`}
          target="_blank"
          variant="outlined"
        >
          Update
        </Button>
      </AccordionDetails>
    </Accordion>
  );
};
