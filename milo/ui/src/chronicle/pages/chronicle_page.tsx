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
  FailedEnvironment,
} from '../components/context';
import { EnvironmentSelectorDialog } from '../components/environment_selector_dialog';
import { FailedEnvironmentsList } from '../components/failed_environments_list';
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

function CenteredContainer({ children }: { children: React.ReactNode }) {
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
      {children}
    </Box>
  );
}

function FailedEnvironmentsSection({
  failedEnvironments,
}: {
  failedEnvironments: readonly FailedEnvironment[];
}) {
  if (failedEnvironments.length === 0) {
    return null;
  }
  return (
    <Box
      sx={{
        mt: 2,
        maxWidth: 600,
        width: '100%',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'flex-start',
      }}
    >
      <Typography
        variant="body2"
        color="warning.main"
        align="left"
        sx={{ fontWeight: 'medium', mb: 0.5 }}
      >
        Warning: The following environments could not be checked due to
        timeouts/errors:
      </Typography>
      <FailedEnvironmentsList failedEnvironments={failedEnvironments} />
    </Box>
  );
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
    failedEnvironments.some((f) => f.errorType === DetectionErrorType.NoAccess);

  const formattedWorkplanId = workplanId ? formatWorkplanUrlId(workplanId) : '';

  const handleDialogClose = () => {
    setDetectionCancelled(true);
    setShowEnvDialog(false);
    setDetecting(false);
  };

  if (detecting) {
    return (
      <CenteredContainer>
        <CircularProgress />
        <Typography>
          Detecting the Turbo CI instance that contains workplan {workplanId}.
        </Typography>
      </CenteredContainer>
    );
  }

  if (detectionCancelled) {
    return (
      <CenteredContainer>
        <Typography variant="h5" color="warning.main">
          Selection Cancelled
        </Typography>
        <Typography color="text.secondary" align="center">
          Environment selection was cancelled. To view the workplan, you must
          select an environment.
        </Typography>
        <FailedEnvironmentsSection failedEnvironments={failedEnvironments} />
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
      </CenteredContainer>
    );
  }

  if (detectionFailed) {
    return (
      <CenteredContainer>
        {hasAccessDenied && isAnonymous ? (
          <Alert severity="warning" sx={{ maxWidth: 600 }}>
            <AlertTitle>Authentication Required</AlertTitle>
            You are not logged in. Please{' '}
            <Link
              href={getLoginUrl(
                location.pathname + location.search + location.hash,
              )}
            >
              log in
            </Link>{' '}
            and try again.
          </Alert>
        ) : (
          <>
            <Typography variant="h5" color="error">
              {hasAccessDenied ? 'Access Denied' : 'Workplan Not Found'}
            </Typography>
            <Typography color="text.secondary" align="center">
              {hasAccessDenied
                ? `Access was denied to one or more Turbo CI environments when searching for workplan ${workplanId}.`
                : `Workplan ${workplanId} could not be found in any of the Turbo CI environments.`}
            </Typography>
            <FailedEnvironmentsSection
              failedEnvironments={failedEnvironments}
            />
          </>
        )}
      </CenteredContainer>
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
