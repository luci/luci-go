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

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import { ChromeOSRepairTable } from './chromeos_repair_table';
import { PriorityRulesPanel } from './priority_rules_panel';
import { useRepairQueueColumns } from './use_repair_queue_columns';

export const ChromeOSRepairDashboard = () => {
  const { columns } = useRepairQueueColumns();

  return (
    <div
      css={{
        margin: '24px',
        paddingBottom: '40px',
      }}
    >
      <div css={{ marginBottom: '16px' }}>
        <Typography variant="h4">ChromeOS Manual Repair Dashboard</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          Rule-based Priority Scoring using dynamic interactive FCon filter Bars
          with Range Filters.
        </Typography>
      </div>

      <Box
        sx={{
          display: 'flex',
          flexWrap: 'wrap',
          width: '100%',
          gap: 3,
        }}
      >
        <Box
          sx={{
            flex: '1 1 540px',
            minWidth: 0,
          }}
        >
          <PriorityRulesPanel />
        </Box>
        <Box
          sx={{
            flex: '1 1 360px',
            minWidth: 0,
          }}
        >
          <Typography variant="h6" sx={{ mt: 2, mb: 2, fontSize: '16px' }}>
            Active IRM Bugs
          </Typography>
          <h1>TODO</h1>
        </Box>
      </Box>

      <Divider sx={{ mt: 4, mb: 4 }} />

      <Typography variant="h6" sx={{ fontSize: '16px' }}>
        Prioritized Manual Repair Queue
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
        Calculated per-device queue, automatically sorted by total score. Hover
        over the score to see exact matched filters breakdown.
      </Typography>

      <div css={{ marginTop: 16 }}>
        <ChromeOSRepairTable columns={columns} filter={''} />
      </div>
    </div>
  );
};

export const ChromeOsRepairDashboard = ChromeOSRepairDashboard;

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-chromeos-repairs">
      <FleetHelmet pageTitle="ChromeOS Repairs" />
      <RecoverableErrorBoundary key="fleet-chromeos-repairs">
        <LoggedInBoundary>
          <ChromeOSRepairDashboard />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
