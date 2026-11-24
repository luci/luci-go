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

import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import {
  Alert,
  Box,
  Button,
  Snackbar,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import { ReactNode, useState } from 'react';

import { useFeatureFlag } from '@/common/feature_flags';
import {
  enableWorkforceInProgressRepairs,
  enableWorkforcePickupRank,
  enableWorkforceTimeframeFilter,
} from '@/fleet/features';
import { getErrorMessage } from '@/fleet/utils/errors';

import {
  DEFAULT_WORKFORCE_LABELS,
  TimeframeOption,
  WorkforceActivityData,
  WorkforceActivityLabels,
} from './workforce_helpers';
import { WorkforceKpiCards } from './workforce_kpi_cards';
import { WorkforceTechniciansTable } from './workforce_technicians_table';

const TIMEFRAME_OPTIONS: readonly { value: TimeframeOption; label: string }[] =
  [
    { value: '1D', label: 'Last Day (1D)' },
    { value: '7D', label: 'Last Week (7D)' },
    { value: '30D', label: 'Last Month (30D)' },
    { value: 'YTD', label: 'YTD' },
  ];

export interface SharedWorkforceActivityViewProps {
  data?: WorkforceActivityData;
  isPending: boolean;
  isError: boolean;
  error: unknown;
  timeframe: TimeframeOption;
  onTimeframeChange: (timeframe: TimeframeOption) => void;
  onBackToQueue: () => void;
  onUnclaimTask?: (taskId: string) => Promise<unknown>;
  isUnclaimPending?: boolean;
  headerActions?: ReactNode;
  labels?: Partial<WorkforceActivityLabels>;
}

export const SharedWorkforceActivityView = ({
  data,
  isPending,
  isError,
  error,
  timeframe,
  onTimeframeChange,
  onBackToQueue,
  onUnclaimTask,
  isUnclaimPending = false,
  headerActions,
  labels,
}: SharedWorkforceActivityViewProps) => {
  const [unclaimError, setUnclaimError] = useState<string | null>(null);
  const showPickupRank = useFeatureFlag(enableWorkforcePickupRank);
  const showInProgressRepairs = useFeatureFlag(
    enableWorkforceInProgressRepairs,
  );
  const showTimeframeFilter = useFeatureFlag(enableWorkforceTimeframeFilter);
  const resolvedLabels: WorkforceActivityLabels = {
    ...DEFAULT_WORKFORCE_LABELS,
    ...labels,
  };

  const handleUnclaimTask = onUnclaimTask
    ? (taskId: string) => {
        void onUnclaimTask(taskId).catch((err) => {
          setUnclaimError(getErrorMessage(err, 'unclaim repair task'));
        });
      }
    : undefined;
  const clearUnclaimError = () => setUnclaimError(null);

  return (
    <Box sx={{ containerType: 'inline-size' }}>
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
          flexWrap: 'nowrap',
          gap: 2,
          mb: 2.5,
          '@container (max-width: 900px)': {
            flexDirection: 'column',
            alignItems: 'stretch !important',
          },
        }}
      >
        <Box sx={{ flex: '1 1 0%', minWidth: 0 }}>
          <Typography variant="h4" sx={{ fontWeight: 700 }}>
            Workforce &amp; Technician Monitoring
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            Real-time technician throughput tracking, active repair assignments,
            and average performance metrics.
          </Typography>
        </Box>

        <Box
          sx={{
            flex: '0 0 auto',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'flex-end',
            gap: 1.5,
            '@container (max-width: 900px)': {
              width: '100%',
              flexDirection: 'row !important',
              flexWrap: 'wrap',
              justifyContent: 'space-between',
              alignItems: 'center !important',
              gap: '12px !important',
            },
            '@container (max-width: 730px)': { gap: '8px !important' },
          }}
        >
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 2,
              flexWrap: 'wrap',
            }}
          >
            {headerActions}
            <Button
              variant="toolbar"
              startIcon={<ArrowBackIcon />}
              onClick={onBackToQueue}
            >
              Prioritized Manual Repair Queue
            </Button>
          </Box>

          {showTimeframeFilter && (
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                flexWrap: 'wrap',
                gap: 1,
                '@container (max-width: 730px)': { gap: '6px !important' },
              }}
            >
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{
                  fontWeight: 600,
                  '@container (max-width: 730px)': {
                    fontSize: '0.75rem !important',
                  },
                }}
              >
                Timeframe:
              </Typography>
              <ToggleButtonGroup
                value={timeframe}
                exclusive
                size="small"
                onChange={(_, newValue: TimeframeOption | null) => {
                  if (newValue) onTimeframeChange(newValue);
                }}
                sx={{
                  '& .MuiToggleButton-root': {
                    textTransform: 'none',
                    px: 1.25,
                    py: 0.5,
                    fontSize: '0.8125rem',
                    whiteSpace: 'nowrap',
                  },
                  '@container (max-width: 730px)': {
                    '& .MuiToggleButton-root': {
                      padding: '3px 6px !important',
                      fontSize: '0.75rem !important',
                    },
                  },
                }}
              >
                {TIMEFRAME_OPTIONS.map(({ value, label }) => (
                  <ToggleButton key={value} value={value}>
                    {label}
                  </ToggleButton>
                ))}
              </ToggleButtonGroup>
            </Box>
          )}
        </Box>
      </Box>

      {isError && (
        <Alert
          severity="error"
          sx={{ mb: 2.5 }}
          data-testid="workforce-activity-error"
        >
          {getErrorMessage(error, 'loading workforce activity data')}
        </Alert>
      )}

      <WorkforceKpiCards
        data={data}
        loading={isPending}
        timeframe={timeframe}
        showPickupRank={showPickupRank}
        showInProgressRepairs={showInProgressRepairs}
        inProgressSubtitle={resolvedLabels.inProgressSubtitle}
      />

      <WorkforceTechniciansTable
        technicians={data?.technicians ?? []}
        timeframe={timeframe}
        loading={isPending}
        isError={isError}
        showPickupRank={showPickupRank}
        showInProgressRepairs={showInProgressRepairs}
        onUnclaimTask={handleUnclaimTask}
        isUnclaimPending={isUnclaimPending}
        labels={resolvedLabels}
      />

      <Snackbar
        open={Boolean(unclaimError)}
        autoHideDuration={6000}
        onClose={clearUnclaimError}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          onClose={clearUnclaimError}
          severity="error"
          sx={{ width: '100%' }}
        >
          {unclaimError}
        </Alert>
      </Snackbar>
    </Box>
  );
};
