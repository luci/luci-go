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

import GroupIcon from '@mui/icons-material/Group';
import { Box, Button, Chip, Divider, Typography } from '@mui/material';
import { useMemo } from 'react';
import { useNavigate } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFeatureFlag } from '@/common/feature_flags';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import {
  CHROMEOS_PLATFORM,
  generateRepairsWorkforceURL,
  platformToURL,
} from '@/fleet/constants/paths';
import { enableChromeOsWorkforceActivity } from '@/fleet/features';
import { useCurrentPlatform } from '@/fleet/hooks/usePlatform';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { ListRepairQueueRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { ActiveIrmTable } from './active_irm_table';
import { ChromeOSRepairTable } from './chromeos_repair_table';
import { PriorityRulesPanel } from './priority_rules_panel';
import { useRepairQueue } from './use_repair_queue';

export const ChromeOSRepairDashboard = () => {
  const navigate = useNavigate();
  const currentPlatform = useCurrentPlatform();
  const isWorkforceEnabled = useFeatureFlag(enableChromeOsWorkforceActivity);

  const request = useMemo(
    () =>
      ListRepairQueueRequest.fromPartial({
        pageSize: 100,
        pageToken: '',
      }),
    [],
  );

  const queueQuery = useRepairQueue(request);

  const inProgressCount = queueQuery.data?.inProgressCount ?? 0;

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
          <Typography variant="h4">ChromeOS Manual Repair Dashboard</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            Rule-based Priority Scoring using dynamic interactive FCon filter
            Bars with Range Filters.
          </Typography>
        </div>

        {isWorkforceEnabled && (
          <Button
            variant="outlined"
            onClick={() =>
              navigate(
                generateRepairsWorkforceURL(
                  currentPlatform
                    ? platformToURL(currentPlatform)
                    : CHROMEOS_PLATFORM,
                ),
              )
            }
            sx={{
              textTransform: 'none',
              fontWeight: 600,
              borderRadius: '8px',
              px: 2,
              py: 0.75,
              display: 'flex',
              alignItems: 'center',
              gap: 1,
            }}
          >
            <GroupIcon sx={{ fontSize: 20 }} />
            <span>Workforce Activity</span>
            <Chip
              label={`${inProgressCount} In-Progress`}
              size="small"
              sx={{
                ml: 0.5,
                bgcolor: 'primary.main',
                color: 'primary.contrastText',
                fontWeight: 600,
                height: 22,
                fontSize: '0.75rem',
              }}
            />
          </Button>
        )}
      </Box>

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
          <ActiveIrmTable />
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
        <ChromeOSRepairTable />
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
