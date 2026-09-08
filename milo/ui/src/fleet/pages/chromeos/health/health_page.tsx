// Copyright 2026 The LUCI Authors.
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

import { ArrowBack, Settings } from '@mui/icons-material';
import { Box, Button, Typography } from '@mui/material';

import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { ExpectedQuotaCard } from './expected_quota_card';

export const HealthPage = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const pageTab: 'overview' | 'configuration' =
    searchParams.get('tab') === 'configuration' ? 'configuration' : 'overview';

  const setPageTab = (newTab: 'overview' | 'configuration') => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (newTab === 'overview') {
        next.delete('tab');
      } else {
        next.set('tab', newTab);
      }
      return next;
    });
  };

  return (
    <Box sx={{ p: 4, minHeight: '100vh' }}>
      <FleetHelmet
        pageTitle={`ChromeOS Fleet Health Metrics${
          pageTab === 'overview' ? '' : ' - Configuration'
        }`}
      />
      <Box sx={{ maxWidth: 1400, mx: 'auto' }}>
        {/* Initial Dashboard View: Only Title, Subtitle, and Top-Right Configuration Button */}
        {pageTab === 'overview' && (
          <Box
            sx={{
              mb: 4,
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'flex-start',
              flexWrap: 'wrap',
              gap: 2,
            }}
          >
            <Box>
              <Typography
                variant="h4"
                component="h1"
                gutterBottom
                sx={{ fontWeight: 'bold' }}
              >
                ChromeOS Fleet Health Metrics
              </Typography>
              <Typography variant="subtitle1" color="text.secondary">
                Analyze hardware reliability trends, monitor model health, and
                identify pool degradations.
              </Typography>
            </Box>
            <Box
              sx={{
                display: 'flex',
                gap: 1.5,
                alignItems: 'center',
                flexWrap: 'wrap',
              }}
            >
              <Button
                variant="outlined"
                color="primary"
                startIcon={<Settings />}
                onClick={() => setPageTab('configuration')}
                sx={{
                  textTransform: 'none',
                  fontWeight: 'bold',
                  px: 2,
                  py: 0.8,
                  borderRadius: 1.5,
                }}
              >
                Configuration
              </Button>
            </Box>
          </Box>
        )}

        {/* Configuration View: Back button, Header, and Expected Quota Card */}
        {pageTab === 'configuration' && (
          <Box sx={{ mb: 4 }}>
            <Button
              startIcon={<ArrowBack />}
              onClick={() => setPageTab('overview')}
              sx={{
                textTransform: 'none',
                fontWeight: 'bold',
                mb: 2,
                color: 'text.secondary',
                '&:hover': { color: 'text.primary' },
              }}
            >
              Back to Fleet Overview
            </Button>
            <Box sx={{ mb: 4 }}>
              <Typography
                variant="h4"
                component="h1"
                gutterBottom
                sx={{ fontWeight: 'bold' }}
              >
                Fleet Health Configuration
              </Typography>
              <Typography variant="subtitle1" color="text.secondary">
                Configure baseline hardware expected fleet quantities.
              </Typography>
            </Box>

            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <ExpectedQuotaCard />
            </Box>
          </Box>
        )}
      </Box>
    </Box>
  );
};

export const Component = HealthPage;
export default HealthPage;
