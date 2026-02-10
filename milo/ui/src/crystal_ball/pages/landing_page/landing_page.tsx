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
import Snackbar from '@mui/material/Snackbar';
import Typography from '@mui/material/Typography';
import { useState } from 'react';
import { Link as RouterLink } from 'react-router';

import { DashboardListTable } from '@/crystal_ball/components/dashboard_list_table/dashboard_list_table';

import { mockDashboards } from './mock_data';

/**
 * A landing page component that displays a list of dashboards.
 */
export function LandingPage() {
  const [toastOpen, setToastOpen] = useState(false);

  const handleFeatureNotReady = () => {
    setToastOpen(true);
  };

  const handleCloseToast = () => {
    setToastOpen(false);
  };

  return (
    <Box sx={{ p: 3 }}>
      <Box
        sx={{
          mb: 3,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}
      >
        <Typography variant="h4" component="h1">
          CrystalBall Dashboards
        </Typography>
        <Box sx={{ display: 'flex', gap: 2 }}>
          <Button variant="outlined" component={RouterLink} to="demo">
            Demo Page
          </Button>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleFeatureNotReady}
          >
            New Dashboard
          </Button>
        </Box>
      </Box>
      <DashboardListTable
        dashboards={mockDashboards}
        onDashboardClick={handleFeatureNotReady}
      />
      <Snackbar
        open={toastOpen}
        autoHideDuration={4000}
        onClose={handleCloseToast}
        message="Feature under construction"
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
