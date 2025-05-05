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

import { Box, ThemeProvider } from '@mui/material';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { gm3PageTheme } from '@/common/themes/gm3_theme';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { TestDetails } from '@/test_investigation/components/test_details';
import { TestInfo } from '@/test_investigation/components/test_info';

export function TestInvestigatePage() {
  return (
    <ThemeProvider theme={gm3PageTheme}>
      <Box
        sx={{
          padding: 2,
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
          gap: '36px',
          bgcolor: 'background.default',
        }}
      >
        <TestInfo />
        <TestDetails />
      </Box>
    </ThemeProvider>
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="test-investigate">
      <RecoverableErrorBoundary key="test-investigate">
        <TestInvestigatePage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
