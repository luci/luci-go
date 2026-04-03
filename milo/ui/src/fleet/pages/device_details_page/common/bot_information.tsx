// Copyright 2026 The LUCI Authors.
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
  AccordionDetails,
  AccordionSummary,
  Alert,
  Button,
  Grid2,
  Link,
  Typography,
} from '@mui/material';
import { ReactNode } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { StandaloneAccordion } from '@/fleet/components/accordion/standalone_accordion';
import { useBotDetails } from '@/fleet/hooks/use_bot_details';
import { BotNotFoundAlert } from '@/fleet/pages/device_details_page/common/bot_not_found_alert';
import { prettyDateTime } from '@/fleet/utils/dates';
import { getErrorMessage } from '@/fleet/utils/errors';
import { getTaskURL } from '@/fleet/utils/swarming';

const quarantineMessage = (state: {
  quarantined: undefined | string | boolean;
  error: string;
}) => {
  let msg = state.quarantined;
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

interface BotInformationProps {
  swarmingHost: string;
  dutId?: string;
  botId?: string;
}

export const BotInformation = ({
  swarmingHost,
  dutId,
  botId,
}: BotInformationProps) => {
  const {
    data: botInfoData,
    isLoading,
    isError,
    error,
    botFound,
    botState,
  } = useBotDetails(swarmingHost, dutId || '', botId || '');

  const currentTaskNode = !botInfoData?.taskId ? (
    'idle'
  ) : (
    <Link
      href={getTaskURL(botInfoData.taskId, swarmingHost)}
      target="_blank"
      rel="noreferrer"
    >
      {botInfoData.taskName || botInfoData.taskId}
    </Link>
  );

  return (
    <StandaloneAccordion>
      <AccordionSummary
        expandIcon={<ExpandMoreIcon />}
        sx={{
          '& .MuiAccordionSummary-content': {
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
          },
        }}
      >
        <Typography variant="h6">Bot information</Typography>
        {botFound && botInfoData?.botId && (
          <div>
            <Button
              color="primary"
              startIcon={<LaunchIcon />}
              size="small"
              href={`https://${swarmingHost}/bot?id=${botInfoData?.botId}`}
              target="_blank"
              onClick={(e) => e.stopPropagation()}
            >
              View in Swarming
            </Button>
          </div>
        )}
      </AccordionSummary>
      <AccordionDetails>
        {!swarmingHost ? (
          <Alert severity="warning">
            Bot information not available (missing swarming instance or bot ID)
          </Alert>
        ) : isError ? (
          <Alert severity="error">
            <div>Unable to load bot information.</div>
            <div>{getErrorMessage(error, 'list bots')}</div>
          </Alert>
        ) : isLoading ? (
          <div
            css={{
              width: '100%',
              margin: '24px 0px',
            }}
          >
            <CentralizedProgress />
          </div>
        ) : botFound ? (
          <>
            <Grid2 container spacing={1} alignItems="center">
              <InfoRow label="Bot ID" value={botInfoData?.botId || ''} />
              {botInfoData?.deleted && <InfoRow label="Deleted" value="True" />}
              {botInfoData?.quarantined && (
                <InfoRow
                  label="Quarantined"
                  value={quarantineMessage(botState)}
                />
              )}
              {botInfoData?.maintenanceMsg && (
                <InfoRow
                  label="In Maintenance"
                  value={botInfoData.maintenanceMsg}
                />
              )}
              {botInfoData?.isDead && !botInfoData?.deleted && (
                <InfoRow
                  label="Status"
                  value="Dead - Bot has been missing longer than 10 minutes"
                />
              )}
              <InfoRow
                label={botInfoData?.isDead ? 'Died on Task' : 'Current Task'}
                value={currentTaskNode}
              />
              <InfoRow
                label="First Seen"
                value={prettyDateTime(botInfoData?.firstSeenTs)}
              />
              <InfoRow
                label="Last Seen"
                value={prettyDateTime(botInfoData?.lastSeenTs)}
              />
            </Grid2>
          </>
        ) : (
          <BotNotFoundAlert dutId={dutId} />
        )}
      </AccordionDetails>
    </StandaloneAccordion>
  );
};
