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

import { Box, Card, Divider, Typography } from '@mui/material';

import { useInvocation } from '@/test_investigation/context';
import { formatDurationSince } from '@/test_investigation/utils/test_info_utils';

import { useAssociatedBugs } from '../context';

import { AnalysisSubsection } from './analysis_subsection';
import { NextStepsSubsection } from './next_steps_subsection';

export function RecommendationsSection() {
  const invocation = useInvocation();
  const associatedBugs = useAssociatedBugs();
  // Data points that were part of uiPlaceholders, now defined directly or passed
  const recommendationCount = associatedBugs.length || 0;
  const lastUpdateTime = invocation.finalizeTime
    ? formatDurationSince(invocation.finalizeTime) || 'N/A'
    : 'N/A';
  // Define any other *real* links or data points that were in uiPlaceholders and are still needed
  return (
    <Box sx={{ flex: { md: 2 } }}>
      <Card
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 2,
          p: 2,
          height: '100%',
        }}
      >
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
          }}
        >
          <Typography variant="h6" component="div">
            Recommendation ({recommendationCount})
          </Typography>
          <Typography variant="caption" color="text.secondary">
            Last update: {lastUpdateTime}
          </Typography>
        </Box>
        <Divider sx={{ mb: 0 }} />
        <Box
          sx={{
            display: 'flex',
            flexDirection: { xs: 'column', sm: 'row' },
            gap: { xs: 2, sm: 3 },
            flexGrow: 1,
          }}
        >
          <AnalysisSubsection />
          <NextStepsSubsection />
        </Box>
      </Card>
    </Box>
  );
}
