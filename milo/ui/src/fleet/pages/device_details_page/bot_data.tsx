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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import LaunchIcon from '@mui/icons-material/Launch';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Grid2,
  Link,
  Typography,
} from '@mui/material';
import { GridColDef } from '@mui/x-data-grid';
import { EditorConfiguration } from 'codemirror';
import { ReactNode, useRef } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { DEFAULT_CODE_MIRROR_CONFIG } from '@/fleet/constants/component_config';
import { useBot } from '@/fleet/hooks/swarming_hooks';
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { prettyDateTime } from '@/fleet/utils/dates';
import { getErrorMessage } from '@/fleet/utils/errors';
import { getTaskURL } from '@/fleet/utils/swarming';
import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';
import { useBotsClient } from '@/swarming/hooks/prpc_clients';

// Copied from go.chromium.org/luci/swarming/server/ui2/modules/bot-page/bot-page-helpers.js
const quarantineMessage = (state: {
  quarantined: undefined | string | boolean;
  error: string;
}) => {
  let msg = state.quarantined;
  // Sometimes, the quarantined message is actually in 'error'.  This
  // happens when the bot code has thrown an exception.
  if (msg === undefined || msg === 'true' || msg === true) {
    msg = state.error;
  }
  return msg || 'True';
};

const InfoRow = ({ label, value }: { label: string; value: ReactNode }) => (
  <>
    <Grid2 size={2}>
      <Typography variant="body2" fontWeight="bold">
        {label}
      </Typography>
    </Grid2>
    <Grid2 size={10}>
      <Typography
        variant="body2"
        fontFamily="monospace"
        sx={{
          whiteSpace: 'nowrap',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
        }}
      >
        {value}
      </Typography>
    </Grid2>
  </>
);

export const BotData = ({
  dutId,
  swarmingHost = DEVICE_TASKS_SWARMING_HOST,
}: {
  dutId: string;
  swarmingHost?: string;
}) => {
  const editorOptions = useRef<EditorConfiguration>(DEFAULT_CODE_MIRROR_CONFIG);
  const client = useBotsClient(swarmingHost);
  const botData = useBot(client, dutId);

  if (botData.isError) {
    return (
      <Alert severity="error">
        {getErrorMessage(botData.error, 'list bots')}{' '}
      </Alert>
    );
  }
  if (botData.isLoading) {
    return (
      <div
        css={{
          width: '100%',
          margin: '24px 0px',
        }}
      >
        <CentralizedProgress />
      </div>
    );
  }
  if (!botData.botFound) {
    return (
      <AlertWithFeedback
        severity="warning"
        title="Bot not found!"
        bugErrorMessage={`Bot not found for device: ${dutId}`}
      >
        <p>
          Oh no! No bots were found for this device (<code>dut_id={dutId}</code>
          ).
        </p>
      </AlertWithFeedback>
    );
  }

  const state = JSON.parse(botData.info?.state || '{}');
  const prettyState = JSON.stringify(state, undefined, 2);

  const dimensionRows =
    botData.info?.dimensions?.map((d, i) => ({
      id: i,
      key: d.key,
      value: d.value.join(', '),
    })) || [];

  const dimensionColumns: GridColDef[] = [
    { field: 'key', headerName: 'Key', flex: 1 },
    { field: 'value', headerName: 'Value', flex: 3 },
  ];

  const currentTaskNode = !botData.info?.taskId ? (
    'idle'
  ) : (
    <Link
      href={getTaskURL(botData.info.taskId, swarmingHost)}
      target="_blank"
      rel="noreferrer"
    >
      {botData.info.taskName || botData.info.taskId}
    </Link>
  );

  return (
    <Box>
      <Card variant="outlined" sx={{ mb: 2 }}>
        <CardContent>
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              mb: 2,
            }}
          >
            <Typography variant="h6">Details</Typography>
            {botData.info?.botId && (
              <Button
                color="primary"
                startIcon={<LaunchIcon />}
                size="small"
                href={`https://${swarmingHost}/bot?id=${botData.info?.botId}`}
                target="_blank"
              >
                View in Swarming
              </Button>
            )}
          </Box>
          <Grid2 container spacing={1} alignItems="center">
            <InfoRow label="Bot ID" value={botData.info?.botId || ''} />
            {botData.info?.deleted && <InfoRow label="Deleted" value="True" />}
            {botData.info?.quarantined && (
              <InfoRow label="Quarantined" value={quarantineMessage(state)} />
            )}
            {botData.info?.maintenanceMsg && (
              <InfoRow
                label="In Maintenance"
                value={botData.info.maintenanceMsg}
              />
            )}
            {botData.info?.isDead && !botData.info?.deleted && (
              <InfoRow
                label="Status"
                value="Dead - Bot has been missing longer than 10 minutes"
              />
            )}
            <InfoRow
              label={botData.info?.isDead ? 'Died on Task' : 'Current Task'}
              value={currentTaskNode}
            />
            <InfoRow
              label="First Seen"
              value={prettyDateTime(botData.info?.firstSeenTs)}
            />
            <InfoRow
              label="Last Seen"
              value={prettyDateTime(botData.info?.lastSeenTs)}
            />
          </Grid2>
        </CardContent>
      </Card>

      <Accordion variant="outlined" sx={{ mb: 2 }}>
        <AccordionSummary expandIcon={<ExpandMoreIcon />}>
          <Typography variant="h6">Dimensions</Typography>
        </AccordionSummary>
        <AccordionDetails>
          <StyledGrid
            disableColumnMenu
            disableColumnFilter
            disableRowSelectionOnClick
            rows={dimensionRows}
            columns={dimensionColumns}
            hideFooterPagination
          />
        </AccordionDetails>
      </Accordion>

      <Accordion variant="outlined">
        <AccordionSummary expandIcon={<ExpandMoreIcon />}>
          <Typography variant="h6">State</Typography>
        </AccordionSummary>
        <AccordionDetails>
          <CodeMirrorEditor
            value={prettyState}
            initOptions={editorOptions.current}
          />
        </AccordionDetails>
      </Accordion>
    </Box>
  );
};
