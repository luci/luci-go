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

import BuildIcon from '@mui/icons-material/Build';
import { Button } from '@mui/material';
import { useState } from 'react';
import { v4 as uuidv4 } from 'uuid';

import { useBuildsClient } from '@/build/hooks/prpc_clients';
import { BatchRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

import AutorepairDialog, { SessionInfo } from './autorepair_dialog';
import {
  autorepairRequestsFromDuts,
  DutNameAndState,
  extractBuildIdentifiers,
} from './shared';

interface RunAutorepairProps {
  selectedDuts: DutNameAndState[];
}

export function RunAutorepair({ selectedDuts }: RunAutorepairProps) {
  const bbClient = useBuildsClient();
  const [open, setOpen] = useState<boolean>(false);
  const [sessionInfo, setSessionInfo] = useState<SessionInfo>({});

  // First, give users a modal to confirm if they want autorepair or not.
  const initializeAutorepair = () => {
    const sessionId = uuidv4();

    setSessionInfo({
      sessionId,
      dutNames: selectedDuts.map((d) => d.name),
    });
    setOpen(true);

    return;
  };

  const runAutorepair = async () => {
    const resp = await bbClient.Batch(
      BatchRequest.fromPartial({
        requests: autorepairRequestsFromDuts(
          selectedDuts || [],
          sessionInfo.sessionId || '',
        ),
      }),
    );

    setSessionInfo({
      ...sessionInfo,
      builds: extractBuildIdentifiers(resp),
    });

    return;
  };

  return (
    <>
      <Button
        color="primary"
        size="small"
        startIcon={<BuildIcon />}
        onClick={initializeAutorepair}
      >
        Run autorepair
      </Button>
      <AutorepairDialog
        sessionInfo={sessionInfo}
        open={open}
        handleClose={() => setOpen(false)}
        handleOk={runAutorepair}
      />
    </>
  );
}
