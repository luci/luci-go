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

import { useAuthState } from '@/common/components/auth_state_provider';
import { useTasks } from '@/fleet/hooks/swarming_hooks';
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { useTasksClient } from '@/swarming/hooks/prpc_clients';

const USER_TAG = 'client_user';
const JOB_TAG = 'task';
const JOB_NAME = 'recovery';
const HOUR_SET_BACK = 24;

export const AutorepairJobsAlert = () => {
  const client = useTasksClient(DEVICE_TASKS_SWARMING_HOST);
  const authState = useAuthState();

  const time_check = useMemo(() => {
    const d = new Date();
    d.setHours(d.getHours() - HOUR_SET_BACK);
    return d;
  }, []);

  const autorepairTags = useMemo(
    () =>
      authState.email
        ? [`${USER_TAG}:${authState.email}`, `${JOB_TAG}:${JOB_NAME}`]
        : [],
    [authState.email],
  );

  const { tasks, isLoading, isError } = useTasks({
    client,
    tags: autorepairTags,
    startTime: time_check.toISOString(),
    limit: 101,
  });

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
          component="a"
          href="admin-tasks"
          variant="text"
          color="primary"
          sx={{ flexShrink: 0, ml: 2 }}
        >
          View Details
        </Button>
      }
    >
      You have {countText} autorepair jobs scheduled in the last 24 hours.
    </Alert>
  );
};
