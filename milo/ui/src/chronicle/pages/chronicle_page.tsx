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

import { Alert, Box, CircularProgress, Typography } from '@mui/material';
import { useContext } from 'react';
import { useParams } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { AppRoutedTab, AppRoutedTabs } from '@/common/components/routed_tabs';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import {
  ChronicleContext,
  DetectionErrorType,
  FailedEnvironment,
} from '../components/context';
import { ChronicleContextProvider } from '../components/provider';

function ChroniclePageContent() {
  const { workplanId, detecting, detectionFailed, failedEnvironments } =
    useContext(ChronicleContext);

  if (detecting) {
    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100vh',
          gap: 2,
        }}
      >
        <CircularProgress />
        <Typography>
          Detecting the Turbo CI instance that contains workplan {workplanId}.
        </Typography>
      </Box>
    );
  }

  if (detectionFailed) {
    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100vh',
          gap: 2,
          p: 3,
        }}
      >
        <Typography variant="h5" color="error">
          Workplan Not Found
        </Typography>
        <Typography color="text.secondary" align="center">
          Workplan {workplanId} could not be found in any of the Turbo CI
          environments.
        </Typography>
        {failedEnvironments.length > 0 && (
          <Typography color="warning.main" align="center" sx={{ mt: 1 }}>
            Note: The following environments could not be checked due to
            timeouts/errors: {formatFailedEnvironments(failedEnvironments)}
          </Typography>
        )}
      </Box>
    );
  }

  return (
    <>
      {failedEnvironments.length > 0 && (
        <Alert severity="warning" sx={{ m: 2, mb: 0 }}>
          The following environments could not be checked due to
          timeouts/errors: {formatFailedEnvironments(failedEnvironments)}
        </Alert>
      )}
      <AppRoutedTabs>
        <AppRoutedTab label="Summary" value="summary" to="summary" />
        <AppRoutedTab label="Timeline" value="timeline" to="timeline" />
        <AppRoutedTab label="Stages & Checks Graph" value="graph" to="graph" />
        <AppRoutedTab label="Tree" value="tree" to="tree" />
        <AppRoutedTab label="Ledger" value="ledger" to="ledger" />
      </AppRoutedTabs>
    </>
  );
}

export function ChroniclePage() {
  const { workplanId } = useParams<{ workplanId: string }>();
  return (
    <ChronicleContextProvider key={workplanId || ''}>
      <ChroniclePageContent />
    </ChronicleContextProvider>
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="chronicle">
      <RecoverableErrorBoundary key="chronicle">
        <ChroniclePage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}

function formatFailedEnvironments(
  failedEnvironments: FailedEnvironment[],
): string {
  return failedEnvironments
    .map((f) => {
      let status: string;
      switch (f.errorType) {
        case DetectionErrorType.Timeout:
          status = 'timed out';
          break;
        case DetectionErrorType.NoAccess:
          status = 'no access';
          break;
        default:
          status = 'failed';
          break;
      }
      return `${f.env.environment} (${status})`;
    })
    .join(', ');
}
