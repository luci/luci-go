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

import AddIcon from '@mui/icons-material/Add';
import { Button, Box, Tab, Tabs } from '@mui/material';
import { useState, useMemo } from 'react';
import { useNavigate } from 'react-router';

import { DashboardDialog, RequireLogin } from '@/crystal_ball/components';
import { DashboardListTable } from '@/crystal_ball/components/dashboard_list_table/dashboard_list_table';
import { useTopBarConfig } from '@/crystal_ball/components/layout/top_bar_context';
import {
  DATA_SPEC_ID,
  GLOBAL_TIME_RANGE_COLUMN,
  GLOBAL_TIME_RANGE_FILTER_ID,
  GLOBAL_TIME_RANGE_OPTION_DEFAULT,
} from '@/crystal_ball/constants';
import { useCreateDashboardState } from '@/crystal_ball/hooks';
import { extractIdFromName, formatApiError } from '@/crystal_ball/utils';
import {
  CreateDashboardStateRequest,
  DashboardState,
  PerfDataSource_SourceType,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * A landing page component that displays a list of dashboards.
 */
export function LandingPage() {
  const navigate = useNavigate();

  const [currentTab, setCurrentTab] = useState(0);
  const [isDialogsOpen, setIsDialogsOpen] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');
  const createMutation = useCreateDashboardState();

  const handleTabChange = (_event: React.SyntheticEvent, newValue: number) => {
    setCurrentTab(newValue);
  };

  const a11yProps = (index: number) => ({
    id: `dashboard-tab-${index}`,
    'aria-controls': `dashboard-tabpanel-${index}`,
  });

  const handleOpenDialog = () => {
    setErrorMsg('');
    setIsDialogsOpen(true);
  };
  const handleCloseDialog = () => setIsDialogsOpen(false);

  const handleCreate = async (data: {
    displayName: string;
    description: string;
  }) => {
    setErrorMsg('');
    try {
      const response = await createMutation.mutateAsync(
        CreateDashboardStateRequest.fromPartial({
          dashboardState: DashboardState.fromPartial({
            displayName: data.displayName,
            description: data.description,
            dashboardContent: {
              widgets: [],
              dataSpecs: {
                [DATA_SPEC_ID]: {
                  displayName: 'Default Data Source',
                  source: { type: PerfDataSource_SourceType.TABLE },
                },
              },
              globalFilters: [
                PerfFilter.fromPartial({
                  id: GLOBAL_TIME_RANGE_FILTER_ID,
                  column: GLOBAL_TIME_RANGE_COLUMN,
                  displayName: 'Time Range (UTC)',
                  range: {
                    defaultValue: {
                      values: [GLOBAL_TIME_RANGE_OPTION_DEFAULT],
                      filterOperator: PerfFilterDefault_FilterOperator.IN_PAST,
                    },
                  },
                }),
              ],
            },
          }),
        }),
      );

      const parsedResp = response.response;
      const newName = parsedResp?.name;
      handleCloseDialog();
      if (newName) {
        const newId = extractIdFromName(newName);
        navigate(`/ui/labs/crystal-ball/dashboards/${newId}`);
      }
    } catch (e) {
      setErrorMsg(formatApiError(e, 'Failed to create dashboard'));
    }
  };

  const handleDashboardClick = (dashboard: DashboardState) => {
    if (dashboard.name) {
      const dashboardId = extractIdFromName(dashboard.name);
      navigate(`/ui/labs/crystal-ball/dashboards/${dashboardId}`);
    }
  };

  const actions = useMemo(
    () => (
      <Button
        variant="contained"
        startIcon={<AddIcon />}
        onClick={handleOpenDialog}
      >
        New Dashboard
      </Button>
    ),
    [],
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
        isPending={createMutation.isPending}
        errorMsg={errorMsg}
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
