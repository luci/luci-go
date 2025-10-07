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
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  ScheduleAutorepairRequest_AutorepairFlag,
  ScheduleAutorepairRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { DutToRepair } from '../../actions/shared/types';

import AutorepairDialog, { SessionInfo } from './autorepair_dialog';

interface RunAutorepairProps {
  selectedDuts: DutToRepair[];
}

export function RunAutorepair({ selectedDuts }: RunAutorepairProps) {
  const fleetConsoleClient = useFleetConsoleClient();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState<boolean>(false);
  const [sessionInfo, setSessionInfo] = useState<SessionInfo>({});
  const [loading, setLoading] = useState<boolean>(false);
  const [deepRepair, setDeepRepair] = useState<boolean>(false);
  const [latestRepair, setLatestRepair] = useState<boolean>(false);
  const dutNames = selectedDuts.map((d) => d.name);

  // First, give users a modal to confirm if they want autorepair or not.
  const initializeAutorepair = () => {
    setSessionInfo({
      dutNames: dutNames,
    });
    setOpen(true);

    return;
  };

  const runAutorepair = async () => {
    setLoading(true);
    const flags = [];
    if (deepRepair) {
      flags.push(ScheduleAutorepairRequest_AutorepairFlag.DEEP_REPAIR);
    }
    if (latestRepair) {
      flags.push(ScheduleAutorepairRequest_AutorepairFlag.LATEST);
    }

    const resp = await fleetConsoleClient.ScheduleAutorepair(
      ScheduleAutorepairRequest.fromPartial({
        unitNames: dutNames,
        flags: flags,
      }),
    );

    setSessionInfo({
      ...sessionInfo,
      sessionId: resp.sessionId,
      results: [...resp.results],
    });
    setLoading(false);

    return;
  };

  const handleClose = () => {
    setOpen(false);
    if (sessionInfo.results) {
      void queryClient.invalidateQueries({ queryKey: ['tasks'] });
    }
    setSessionInfo({});
    setDeepRepair(false);
    setLatestRepair(false);
  };

  return (
    <>
      <Button
        color="primary"
        size="small"
        startIcon={<BuildIcon />}
        onClick={initializeAutorepair}
        disabled={dutNames.length === 0}
      >
        Run autorepair
      </Button>
      <AutorepairDialog
        sessionInfo={sessionInfo}
        open={open}
        handleClose={handleClose}
        handleOk={runAutorepair}
        deepRepair={deepRepair}
        handleDeepRepairChange={(checked) => setDeepRepair(checked)}
        latestRepair={latestRepair}
        handleLatestRepairChange={(checked) => setLatestRepair(checked)}
        loading={loading}
      />
    </>
  );
}
