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

import { Alert, Button } from '@mui/material';
import { useMemo } from 'react';
import { Link } from 'react-router';

import { useAuthState } from '@/common/components/auth_state_provider';
import {
  generateAdminTasksURL,
  CHROMEOS_PLATFORM,
} from '@/fleet/constants/paths';
import { useTasks } from '@/fleet/hooks/swarming_hooks';
import { useAdminTaskTags } from '@/fleet/hooks/use_pending_admin_tasks';
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { useTasksClient } from '@/swarming/hooks/prpc_clients';

const HOUR_SET_BACK = 24;

export const AdminTasksAlert = () => {
  const client = useTasksClient(DEVICE_TASKS_SWARMING_HOST);
  const authState = useAuthState();

  const time_check = useMemo(() => {
    const d = new Date();
    d.setHours(d.getHours() - HOUR_SET_BACK);
    return d;
  }, []);

  const adminTaskTags = useAdminTaskTags();

  const { tasks, isLoading, isError } = useTasks(
    {
      client,
      tags: adminTaskTags,
      startTime: time_check.toISOString(),
      limit: 101,
    },
    { enabled: adminTaskTags.length > 0 },
  );

  const jobsCount = tasks?.length || 0;

  if (isLoading || isError || !authState.email || jobsCount === 0) {
    return null;
  }
  const countText = jobsCount > 100 ? 'over 100' : `${jobsCount}`;

  return (
    <Alert
      severity="info"
      sx={{
        marginTop: '24px',
        alignItems: 'center',
      }}
      action={
        <Button
          component={Link}
          to={generateAdminTasksURL(CHROMEOS_PLATFORM)}
          variant="text"
          color="primary"
          sx={{ flexShrink: 0, ml: 2 }}
        >
          View Details
        </Button>
      }
    >
      You have {countText} admin tasks scheduled in the last 24 hours.
    </Alert>
  );
};
