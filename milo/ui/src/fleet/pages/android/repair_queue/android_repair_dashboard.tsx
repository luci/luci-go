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

import { Box, Typography } from '@mui/material';

import { AndroidPageWorkspace } from '@/fleet/workspaces';

import { AndroidRepairQueueTable } from './android_repair_queue_table';

export interface AndroidRepairDashboardProps {
  workspace: AndroidPageWorkspace;
}

export const AndroidRepairDashboard = ({
  workspace,
}: AndroidRepairDashboardProps) => {
  return (
    <div
      css={{
        margin: '24px',
        paddingBottom: '40px',
      }}
    >
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
          flexWrap: 'wrap',
          gap: 2,
          mb: 2,
        }}
      >
        <div>
          <Typography variant="h4">
            {workspace} Manual Repair Dashboard
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            Manual repair tasks for {workspace} devices.
          </Typography>
        </div>
      </Box>

      <div css={{ marginTop: 16 }}>
        <AndroidRepairQueueTable workspace={workspace} />
      </div>
    </div>
  );
};
