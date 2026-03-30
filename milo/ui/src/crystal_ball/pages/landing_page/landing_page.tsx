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

import { Add as AddIcon } from '@mui/icons-material';
import { Button, Box, Tab, Tabs } from '@mui/material';
import { useState, useMemo, useCallback } from 'react';
import { useNavigate } from 'react-router';

import {
  DashboardDialog,
  DashboardListTable,
  RequireLogin,
  useTopBarConfig,
} from '@/crystal_ball/components';
import { useCreateDashboardWorkflow } from '@/crystal_ball/hooks';
import { CRYSTAL_BALL_ROUTES } from '@/crystal_ball/routes';
import { extractIdFromName } from '@/crystal_ball/utils';
import { DashboardState } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * A landing page component that displays a list of dashboards.
 */
export function LandingPage() {
  const navigate = useNavigate();

  const [currentTab, setCurrentTab] = useState(0);
  const [isDialogsOpen, setIsDialogsOpen] = useState(false);

  const {
    createDashboard,
    isPending: isCreating,
    errorMsg: createErrorMsg,
    setErrorMsg: setCreateErrorMsg,
  } = useCreateDashboardWorkflow();

  const handleTabChange = (_event: React.SyntheticEvent, newValue: number) => {
    setCurrentTab(newValue);
  };

  const a11yProps = (index: number) => ({
    id: `dashboard-tab-${index}`,
    'aria-controls': `dashboard-tabpanel-${index}`,
  });

  const handleOpenDialog = useCallback(() => {
    setCreateErrorMsg('');
    setIsDialogsOpen(true);
  }, [setCreateErrorMsg]);

  const handleCloseDialog = useCallback(
    () => setIsDialogsOpen(false),
    [setIsDialogsOpen],
  );

  const handleCreate = async (data: {
    displayName: string;
    description: string;
  }) => {
    await createDashboard(data, handleCloseDialog);
  };

  const handleDashboardClick = (dashboard: DashboardState) => {
    if (dashboard.name) {
      const dashboardId = extractIdFromName(dashboard.name);
      navigate(CRYSTAL_BALL_ROUTES.DASHBOARD_DETAIL(dashboardId));
    }
  };

  const actions = useMemo(
    () => (
      <Box sx={{ display: 'flex', gap: 2 }}>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleOpenDialog}
        >
          New Dashboard
        </Button>
      </Box>
    ),
    [handleOpenDialog],
  );

  useTopBarConfig(null, actions);

  return (
    <Box sx={{ p: 3 }}>
      <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
        <Tabs
          value={currentTab}
          onChange={handleTabChange}
          aria-label="dashboard list tabs"
        >
          <Tab label="Active Dashboards" {...a11yProps(0)} />
          <Tab label="Deleted Dashboards" {...a11yProps(1)} />
        </Tabs>
      </Box>
      <Box
        role="tabpanel"
        id={`dashboard-tabpanel-${currentTab}`}
        aria-labelledby={`dashboard-tab-${currentTab}`}
      >
        <DashboardListTable
          onDashboardClick={handleDashboardClick}
          showDeleted={currentTab === 1}
        />
      </Box>
      <DashboardDialog
        open={isDialogsOpen}
        onClose={handleCloseDialog}
        onSubmit={handleCreate}
        isPending={isCreating}
        errorMsg={createErrorMsg}
      />
    </Box>
  );
}

/**
 * Component export for Landing Page.
 */
export function Component() {
  return (
    <RequireLogin>
      <LandingPage />
    </RequireLogin>
  );
}
