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

import SystemUpdateAltIcon from '@mui/icons-material/SystemUpdateAlt';
import { Button } from '@mui/material';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { ScheduleDeployRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { AdminAccessRequiredDialog } from '../shared/admin_access_required_dialog';
import { DutToRepair } from '../shared/types';
import { useAdminTaskPermission } from '../shared/use_admin_task_permission';

import DeployDialog, { SessionInfo } from './deploy_dialog';

interface RunDeployProps {
  selectedDuts: DutToRepair[];
}

export function RunDeploy({ selectedDuts }: RunDeployProps) {
  const { trackEvent } = useGoogleAnalytics();
  const fleetConsoleClient = useFleetConsoleClient();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState<boolean>(false);
  const [sessionInfo, setSessionInfo] = useState<SessionInfo>({});
  const [loading, setLoading] = useState<boolean>(false);
  const [adminAccessRequiredDialogOpen, setAdminAccessRequiredDialogOpen] =
    useState<boolean>(false);

  const hasPermission = useAdminTaskPermission();

  const dutNames = selectedDuts.map((d) => d.name);
  const namespaces = selectedDuts.map((d) => d.namespace || '');

  const initializeDeploy = () => {
    if (hasPermission === false) {
      setAdminAccessRequiredDialogOpen(true);
      return;
    }
    setSessionInfo({
      dutNames: dutNames,
      namespaces: namespaces,
    });
    setOpen(true);
    return;
  };

  const runDeploy = async () => {
    trackEvent('run_deploy', {
      componentName: 'run_deploy_button',
      dutCount: selectedDuts.length,
    });
    setLoading(true);

    const resp = await fleetConsoleClient.ScheduleDeploy(
      ScheduleDeployRequest.fromPartial({
        unitNames: dutNames,
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
  };

  return (
    <>
      <Button
        color="primary"
        size="small"
        startIcon={<SystemUpdateAltIcon />}
        onClick={initializeDeploy}
        disabled={dutNames.length === 0}
      >
        Deploy
      </Button>
      <DeployDialog
        sessionInfo={sessionInfo}
        open={open}
        handleClose={handleClose}
        handleOk={runDeploy}
        loading={loading}
      />
      <AdminAccessRequiredDialog
        open={adminAccessRequiredDialogOpen}
        onClose={() => setAdminAccessRequiredDialogOpen(false)}
      />
    </>
  );
}
