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

import {
  Alert,
  AlertTitle,
  Box,
  Button,
  CircularProgress,
  Link,
  Typography,
} from '@mui/material';
import { useContext } from 'react';
import { useParams, useLocation } from 'react-router';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { AppRoutedTab, AppRoutedTabs } from '@/common/components/routed_tabs';
import { getLoginUrl } from '@/common/tools/url_utils';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import {
  ChronicleContext,
  DetectionErrorType,
  formatFailedEnvironments,
} from '../components/context';
import { EnvironmentSelectorDialog } from '../components/environment_selector_dialog';
import { ChronicleContextProvider } from '../components/provider';
import {
  fromString,
  root,
  toString as idToString,
  workplan,
} from '../utils/id';

function formatWorkplanUrlId(idStr: string): string {
  try {
    const parsed = fromString(idStr);
    const wpId = root(parsed).wp?.id;
    if (wpId) {
      return idToString(workplan(wpId));
    }
  } catch {
    // ignore
  }
  try {
    return idToString(workplan(idStr));
  } catch {
    return idStr;
  }
}

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

  const location = useLocation();
  const authState = useAuthState();

  const isAnonymous =
    !authState.identity || authState.identity === ANONYMOUS_IDENTITY;
  const hasAccessDenied =
    failedEnvironments.length > 0 &&
    failedEnvironments.every(
      (f) => f.errorType === DetectionErrorType.NoAccess,
    );

  const formattedWorkplanId = workplanId ? formatWorkplanUrlId(workplanId) : '';

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
        {hasAccessDenied && isAnonymous && (
          <Alert severity="warning" sx={{ mt: 1, maxWidth: 600 }}>
            <AlertTitle>Authentication Required</AlertTitle>
            Access was denied for this workplan, and you are not currently
            logged in, which may be why access was denied. Consider{' '}
            <Link
              href={getLoginUrl(
                location.pathname + location.search + location.hash,
              )}
            >
              logging in
            </Link>{' '}
            and try again.
          </Alert>
        )}
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
    <Box sx={{ position: 'relative', width: '100%', height: '100%' }}>
      {workplanId && (
        <Box
          sx={{
            position: 'absolute',
            top: '8px',
            right: '16px',
            zIndex: 10,
          }}
        >
          <Link
            href={`http://go/wp/${formattedWorkplanId}`}
            target="_blank"
            rel="noreferrer"
            sx={{ fontSize: '0.875rem', fontWeight: 500 }}
          >
            Legacy Workplan Viewer
          </Link>
        </Box>
      )}
      <AppRoutedTabs>
        <AppRoutedTab
          label="Summary"
          value="summary"
          to={`summary${location.search}`}
          hideWhenInactive
        />
        <AppRoutedTab
          label="Stages & Checks Graph"
          value="graph"
          to={`graph${location.search}`}
        />
        <AppRoutedTab label="Tree" value="tree" to={`tree${location.search}`} />
        <AppRoutedTab
          label="Timeline"
          value="timeline"
          to={`timeline${location.search}`}
        />
        <AppRoutedTab
          label="Ledger"
          value="ledger"
          to={`ledger${location.search}`}
          hideWhenInactive
        />
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
    </Box>
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
