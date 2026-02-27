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
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import { useState, useMemo } from 'react';
import { useNavigate } from 'react-router';

import { CreateDashboardModal } from '@/crystal_ball/components/create_dashboard_modal';
import { DashboardListTable } from '@/crystal_ball/components/dashboard_list_table/dashboard_list_table';
import { useTopBarConfig } from '@/crystal_ball/components/layout/top_bar_context';
import { DashboardState } from '@/crystal_ball/types';
import { extractIdFromName } from '@/crystal_ball/utils';

/**
 * A landing page component that displays a list of dashboards.
 */
export function LandingPage() {
  const navigate = useNavigate();

  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);

  const handleOpenCreateModal = () => setIsCreateModalOpen(true);
  const handleCloseCreateModal = () => setIsCreateModalOpen(false);

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
        onClick={handleOpenCreateModal}
      >
        New Dashboard
      </Button>
    ),
    [],
  );

  useTopBarConfig(null, actions);

  return (
    <Box sx={{ p: 3 }}>
      <DashboardListTable onDashboardClick={handleDashboardClick} />
      <CreateDashboardModal
        open={isCreateModalOpen}
        onClose={handleCloseCreateModal}
      />
    </Box>
  );
}

/**
 * Component export for Landing Page.
 */
export function Component() {
  return <LandingPage />;
}
