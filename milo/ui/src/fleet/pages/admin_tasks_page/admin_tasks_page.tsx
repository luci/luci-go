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

import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { IconButton, Typography } from '@mui/material';
import { useNavigate } from 'react-router';

import { useAuthState } from '@/common/components/auth_state_provider';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import { TasksTable } from './tasks_table';

export const AdminTasksPage = () => {
  const navigate = useNavigate();
  const authState = useAuthState();
  return (
    <div
      css={{
        width: '100%',
        margin: '24px 0px',
      }}
    >
      <div
        css={{
          margin: '0px 24px 8px 24px',
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          gap: 8,
        }}
      >
        <IconButton onClick={() => navigate(-1)}>
          <ArrowBackIcon />
        </IconButton>
        <Typography variant="h4" sx={{ whiteSpace: 'nowrap' }}>
          Tasks history: {authState.email}
        </Typography>
      </div>
      <div css={{ padding: '0 24px' }}>
        <TasksTable />
      </div>
    </div>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-admin-tasks">
      <RecoverableErrorBoundary key="fleet-admin-tasks-page">
        <FleetHelmet pageTitle="Autorepair history" />
        <LoggedInBoundary>
          <AdminTasksPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
