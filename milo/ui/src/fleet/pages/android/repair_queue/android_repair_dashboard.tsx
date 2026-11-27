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
import { useMemo } from 'react';
import { useNavigate } from 'react-router';

import { useFeatureFlag } from '@/common/feature_flags';
import { WorkforceActivityButton } from '@/fleet/components/repair_queue/workforce_activity_button';
import { generateRepairsWorkforceURL } from '@/fleet/constants/paths';
import { enableAndroidWorkforceActivity } from '@/fleet/features';
import { surfaceCardSx } from '@/fleet/theme/recipes';
import { AndroidPageWorkspace, workspaces } from '@/fleet/workspaces';
import { ListAndroidRepairQueueRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { AndroidMyRepairTasksActions } from './android_my_repair_tasks_actions';
import { AndroidPriorityRulesPanel } from './android_priority_rules_panel';
import { AndroidRepairQueueTable } from './android_repair_queue_table';
import { toAndroidWorkspaceEnum } from './android_workspace';
import { useAndroidRepairQueue } from './use_android_repair_queue';

export interface AndroidRepairDashboardProps {
  workspace: AndroidPageWorkspace;
}

export const AndroidRepairDashboard = ({
  workspace,
}: AndroidRepairDashboardProps) => {
  const navigate = useNavigate();
  const isWorkforceEnabled = useFeatureFlag(enableAndroidWorkforceActivity);
  const queueRequest = useMemo(
    () =>
      ListAndroidRepairQueueRequest.fromPartial({
        pageSize: 100,
        workspace: toAndroidWorkspaceEnum(workspace),
      }),
    [workspace],
  );
  const { inProgressCount } = useAndroidRepairQueue(queueRequest, {
    enabled: isWorkforceEnabled,
  });

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
        {isWorkforceEnabled && (
          <WorkforceActivityButton
            inProgressCount={inProgressCount}
            onClick={() =>
              navigate(
                generateRepairsWorkforceURL(workspaces[workspace].baseUrl),
              )
            }
          />
        )}
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

      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-end',
          flexWrap: 'wrap',
          gap: 2,
        }}
      >
        <div>
          <Typography variant="h6">Prioritized Manual Repair Queue</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            Devices ranked by total priority score. Hover over a score to view
            matching rules.
          </Typography>
        </div>
        <AndroidMyRepairTasksActions workspace={workspace} />
      </Box>

      <div css={{ marginTop: 16 }}>
        <AndroidRepairQueueTable workspace={workspace} />
      </div>
    </div>
  );
};
