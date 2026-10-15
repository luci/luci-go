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

import { Box, Divider, Typography } from '@mui/material';

import { surfaceCardSx } from '@/fleet/theme/recipes';
import { AndroidPageWorkspace } from '@/fleet/workspaces';

import { AndroidPriorityRulesPanel } from './android_priority_rules_panel';
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
            Configure rules and assign points to prioritize devices in the
            repair queue.
          </Typography>
        </div>
      </Box>

      <Box
        sx={{
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'flex-start',
          width: '100%',
          gap: 3,
        }}
      >
        <Box
          sx={{
            flex: '1 1 540px',
            minWidth: 0,
            ...surfaceCardSx,
            px: 2.5,
            py: 2,
          }}
        >
          <AndroidPriorityRulesPanel workspace={workspace} />
        </Box>
      </Box>

      <Divider sx={{ mt: 4, mb: 4 }} />

      <Typography variant="h6">Prioritized Manual Repair Queue</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
        Devices ranked by total priority score. Hover over a score to view
        matching rules.
      </Typography>

      <div css={{ marginTop: 16 }}>
        <AndroidRepairQueueTable workspace={workspace} />
      </div>
    </div>
  );
};
