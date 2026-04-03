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
import {
  AccordionDetails,
  AccordionSummary,
  Alert,
  Typography,
} from '@mui/material';
import { EditorConfiguration } from 'codemirror';
import { useRef } from 'react';

import { StandaloneAccordion } from '@/fleet/components/accordion/standalone_accordion';
import { DEFAULT_CODE_MIRROR_CONFIG } from '@/fleet/constants/component_config';
import { useBotDetails } from '@/fleet/hooks/use_bot_details';
import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';

import { BotNotFoundAlert } from './bot_not_found_alert';

interface BotStateProps {
  swarmingHost: string;
  dutId?: string;
  botId?: string;
}

export const BotState = ({ swarmingHost, dutId, botId }: BotStateProps) => {
  const editorOptions = useRef<EditorConfiguration>(DEFAULT_CODE_MIRROR_CONFIG);
  const { botFound, prettyState } = useBotDetails(
    swarmingHost,
    dutId || '',
    botId || '',
  );

  return (
    <StandaloneAccordion>
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Typography variant="h6">Bot State</Typography>
      </AccordionSummary>
      <AccordionDetails>
        {!swarmingHost ? (
          <Alert severity="warning">
            Bot information not available (missing swarming instance or bot ID)
          </Alert>
        ) : botFound ? (
          <CodeMirrorEditor
            value={prettyState}
            initOptions={editorOptions.current}
          />
        ) : (
          <BotNotFoundAlert dutId={dutId || ''} />
        )}
      </AccordionDetails>
    </StandaloneAccordion>
  );
};
