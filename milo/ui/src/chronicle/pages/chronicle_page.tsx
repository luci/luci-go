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

import { Box, Button, CircularProgress, Typography } from '@mui/material';
import { useContext } from 'react';
import { useParams } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { AppRoutedTab, AppRoutedTabs } from '@/common/components/routed_tabs';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import {
  ChronicleContext,
  formatFailedEnvironments,
} from '../components/context';
import { EnvironmentSelectorDialog } from '../components/environment_selector_dialog';
import { ChronicleContextProvider } from '../components/provider';

function ChroniclePageContent() {
  const {
    workplanId,
    detecting,
    setDetecting,
    detectionFailed,
    showEnvDialog,
    setShowEnvDialog,
    detectedEnvironments,
    setActiveEnvironment,
    requestedEnvFailed,
    failedEnvironments,
    detectionCancelled,
    setDetectionCancelled,
  } = useContext(ChronicleContext);

  const handleDialogClose = () => {
    setDetectionCancelled(true);
    setShowEnvDialog(false);
    setDetecting(false);
  };

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

  if (detectionCancelled) {
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
        <Typography variant="h5" color="warning.main">
          Selection Cancelled
        </Typography>
        <Typography color="text.secondary" align="center">
          Environment selection was cancelled. To view the workplan, you must
          select an environment.
        </Typography>
        {failedEnvironments.length > 0 && (
          <Typography
            color="warning.main"
            align="center"
            variant="body2"
            sx={{ mb: 1 }}
          >
            Note: The following environments could not be checked:{' '}
            {formatFailedEnvironments(failedEnvironments)}
          </Typography>
        )}
        <Button
          variant="contained"
          color="primary"
          onClick={() => {
            setShowEnvDialog(true);
            setDetectionCancelled(false);
          }}
        >
          Select Environment
        </Button>
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
      <AppRoutedTabs>
        <AppRoutedTab label="Summary" value="summary" to="summary" />
        <AppRoutedTab label="Stages & Checks Graph" value="graph" to="graph" />
        <AppRoutedTab label="Tree" value="tree" to="tree" />
        <AppRoutedTab label="Timeline" value="timeline" to="timeline" />
        <AppRoutedTab label="Ledger" value="ledger" to="ledger" />
      </AppRoutedTabs>
      {showEnvDialog && (
        <EnvironmentSelectorDialog
          open={showEnvDialog}
          detectedEnvironments={detectedEnvironments}
          requestedEnvFailed={requestedEnvFailed}
          failedEnvironments={failedEnvironments}
          onSelect={(environment) => {
            setActiveEnvironment(environment);
            setShowEnvDialog(false);
            setDetecting(false);
          }}
          onClose={handleDialogClose}
        />
      )}
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
