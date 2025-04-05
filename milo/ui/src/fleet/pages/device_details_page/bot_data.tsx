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

import LaunchIcon from '@mui/icons-material/Launch';
import { Alert, Box, Button, Typography } from '@mui/material';
import { EditorConfiguration } from 'codemirror';
import { useRef } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import { DEFAULT_CODE_MIRROR_CONFIG } from '@/fleet/constants/component_config';
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { getErrorMessage } from '@/fleet/utils/errors';
import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';
import { useBotsClient } from '@/swarming/hooks/prpc_clients';

import { useBot } from './hooks';

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

  const state = JSON.parse(botData.info?.state || '');
  const prettyState = JSON.stringify(state, undefined, 2);
  return (
    <Box>
      <div css={{ marginBottom: 12, display: 'flex' }}>
        <Typography variant="h4" sx={{ marginRight: 'auto' }}>
          State
        </Typography>
        {botData.info?.botId && (
          <Button
            color="primary"
            startIcon={<LaunchIcon />}
            size="small"
            href={`https://chromeos-swarming.appspot.com/bot?id=${botData.info?.botId}`}
          >
            View in swarming
          </Button>
        )}
      </div>
      <CodeMirrorEditor
        value={prettyState}
        initOptions={editorOptions.current}
      />
    </Box>
  );
};
