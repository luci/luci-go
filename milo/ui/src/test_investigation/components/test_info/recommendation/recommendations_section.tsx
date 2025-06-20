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

import { Box, Card, Typography } from '@mui/material';
import { DateTime } from 'luxon';
import { useMemo } from 'react';

import { useInvocation } from '@/test_investigation/context';

import { AnalysisSubsection } from './analysis_subsection';
import { NextStepsSubsection } from './next_steps_subsection';

// Helper to convert ISO string to Luxon DateTime (can be shared or local)
// Assuming proto timestamps are ISO strings here.
function isoStringToLuxonDateTime(isoString?: string): DateTime | undefined {
  if (!isoString) return undefined;
  const dt = DateTime.fromISO(isoString, { zone: 'utc' });
  return dt.isValid ? dt : undefined;
}

export function RecommendationsSection() {
  const invocation = useInvocation();

  const now = useMemo(() => DateTime.now(), []);
  const lastUpdateTime = useMemo(() => {
    if (!invocation?.finalizeTime) {
      return 'N/A';
    }
    const finalizeDt = isoStringToLuxonDateTime(invocation.finalizeTime);

    if (finalizeDt && finalizeDt.isValid && now.isValid) {
      const formattedDt = `${finalizeDt.hour}:${finalizeDt.minute}, ${finalizeDt.day}/${finalizeDt.month}/${finalizeDt.year} UTC`;
      return formattedDt;
    }
    return 'N/A';
  }, [invocation.finalizeTime, now]);

  return (
    <Box sx={{ flex: { md: 2 } }}>
      <Card
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 2,
          p: 3,
          height: '100%',
          boxSizing: 'border-box',
        }}
      >
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'space-between',
            gap: 0.5,
          }}
        >
          <Typography variant="h6" component="div">
            Suggested next steps
          </Typography>
          <Typography variant="body1" color="text.secondary">
            Last update: {lastUpdateTime}
          </Typography>
        </Box>
        <Box
          sx={{
            display: 'flex',
            flexDirection: { xs: 'column', sm: 'row' },
            gap: { xs: 2, sm: 3 },
            flexGrow: 1,
            alignItems: 'flex-start',
          }}
        >
          <NextStepsSubsection />
          <AnalysisSubsection currentTimeForAgoDt={now} />
        </Box>
      </Card>
    </Box>
  );
}
