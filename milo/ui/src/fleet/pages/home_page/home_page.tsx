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

import ArticleOutlinedIcon from '@mui/icons-material/ArticleOutlined';
import ConstructionIcon from '@mui/icons-material/Construction';
import DevicesIcon from '@mui/icons-material/Devices';
import EmailOutlinedIcon from '@mui/icons-material/EmailOutlined';
import FeedbackIcon from '@mui/icons-material/Feedback';
import { Alert, Box, Button, Grid2, Link, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { useUserProfile } from '@/common/hooks/use_user_profile';
import { getLoginUrl } from '@/common/tools/url_utils';
import { genFeedbackUrl } from '@/common/tools/utils';
import androidLogo from '@/fleet/assets/logos/android.png';
import browserLogo from '@/fleet/assets/logos/browser.png';
import chromeosLogo from '@/fleet/assets/logos/chromeos.png';
import { FEEDBACK_BUGANIZER_BUG_ID } from '@/fleet/constants/feedback';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { PlatformSummaryCard } from './platform_summary_card';

export const HomePage = () => {
  const client = useFleetConsoleClient();
  const { displayFirstName, isAnonymous } = useUserProfile();

  const chromeosQuery = useQuery({
    ...client.CountDevices.query({
      filter: '',
      platform: Platform.CHROMEOS,
    }),
    enabled: !isAnonymous,
  });

  const browserQuery = useQuery({
    ...client.CountBrowserDevices.query({
      filter: '',
    }),
    enabled: !isAnonymous,
  });

  const androidQuery = useQuery({
    ...client.CountDevices.query({
      filter: '',
      platform: Platform.ANDROID,
    }),
    enabled: !isAnonymous,
  });

  const androidRepairsQuery = useQuery({
    ...client.CountRepairMetrics.query({
      filter: '',
      platform: Platform.ANDROID,
    }),
    enabled: !isAnonymous,
  });

  const chromeosTotal = chromeosQuery.data?.total ?? 0;

  const browserTotal = browserQuery.data?.total ?? 0;

  const androidTotal = androidQuery.data?.androidCount?.totalDevices ?? 0;

  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-home">
      <Box
        sx={{
          p: 4,
          display: 'flex',
          flexDirection: 'column',
          minHeight: 'calc(100vh - 64px)',
        }}
      >
        <Box sx={{ maxWidth: 1300, mx: 'auto', width: '100%', mt: 8 }}>
          {/* Hero Section */}
          <Box sx={{ mb: 8, textAlign: 'center' }}>
            <Typography variant="h3" component="h1" gutterBottom>
              Welcome to Fleet Console (FCon)
              {displayFirstName ? `, ${displayFirstName}` : ''}!
            </Typography>
            <Typography
              variant="subtitle1"
              color="text.secondary"
              sx={{ mb: 4, maxWidth: 800, mx: 'auto' }}
            >
              The unified hardware management portal for ChromeOS, Browser, and
              Android testing devices.
            </Typography>
          </Box>

          {/* Fleet Summaries */}

          {isAnonymous ? (
            <Alert severity="info" sx={{ mb: 4 }}>
              You must{' '}
              <Link
                href={getLoginUrl(
                  location.pathname + location.search + location.hash,
                )}
                sx={{ textDecoration: 'underline' }}
              >
                login
              </Link>{' '}
              to load fleet data.
            </Alert>
          ) : (
            <Grid2 container spacing={4}>
              {/* ChromeOS Card */}
              <Grid2 size={{ xs: 12, md: 6, lg: 4 }}>
                <PlatformSummaryCard
                  title="ChromeOS"
                  logoSrc={chromeosLogo}
                  total={chromeosTotal}
                  isLoading={chromeosQuery.isPending}
                  isError={chromeosQuery.isError}
                  linkTo="/ui/fleet/p/chromeos/devices"
                  linkText="View all devices"
                  linkIcon={<DevicesIcon />}
                />
              </Grid2>

              {/* Browser Card */}
              <Grid2 size={{ xs: 12, md: 6, lg: 4 }}>
                <PlatformSummaryCard
                  title="Browser"
                  logoSrc={browserLogo}
                  total={browserTotal}
                  isLoading={browserQuery.isPending}
                  isError={browserQuery.isError}
                  linkTo="/ui/fleet/p/chromium/devices"
                  linkText="View all devices"
                  linkIcon={<DevicesIcon />}
                />
              </Grid2>

              {/* Android Card */}
              <Grid2 size={{ xs: 12, md: 6, lg: 4 }}>
                <PlatformSummaryCard
                  title="Android"
                  logoSrc={androidLogo}
                  total={androidTotal}
                  isLoading={
                    androidQuery.isPending || androidRepairsQuery.isPending
                  }
                  isError={androidQuery.isError}
                  linkTo="/ui/fleet/p/android/devices"
                  linkText="View all devices"
                  linkIcon={<DevicesIcon />}
                  secondaryLinkTo="/ui/fleet/p/android/repairs"
                  secondaryLinkText="View all repairs"
                  secondaryLinkIcon={<ConstructionIcon />}
                  secondTotalText="Devices offline"
                  secondTotal={androidRepairsQuery.data?.offlineDevices}
                />
              </Grid2>
            </Grid2>
          )}

          {/* Useful Resources */}
          <Box sx={{ mt: 10, pt: 4, mb: 10 }}>
            <Box
              sx={{
                display: 'flex',
                flexDirection: 'row',
                flexWrap: 'wrap',
                gap: 2,
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              <Button
                variant="outlined"
                color="inherit"
                component="a"
                href="http://go/fleet-console"
                target="_blank"
                rel="noreferrer"
                startIcon={<ArticleOutlinedIcon />}
                sx={{
                  textTransform: 'none',
                  color: 'text.secondary',
                  borderColor: 'divider',
                  py: 1,
                  px: 2,
                }}
              >
                Learn more
              </Button>
              <Button
                variant="outlined"
                color="inherit"
                component="a"
                href={genFeedbackUrl({
                  bugComponent: FEEDBACK_BUGANIZER_BUG_ID,
                })}
                target="_blank"
                rel="noreferrer"
                startIcon={<FeedbackIcon />}
                sx={{
                  textTransform: 'none',
                  color: 'text.secondary',
                  borderColor: 'divider',
                  py: 1,
                  px: 2,
                }}
              >
                File feedback
              </Button>
              <Button
                variant="outlined"
                color="inherit"
                component="a"
                href="http://g/fleet-console-users"
                target="_blank"
                rel="noreferrer"
                startIcon={<EmailOutlinedIcon />}
                sx={{
                  textTransform: 'none',
                  color: 'text.secondary',
                  borderColor: 'divider',
                  py: 1,
                  px: 2,
                }}
              >
                Subscribe to updates
              </Button>
            </Box>
          </Box>
        </Box>
      </Box>
    </TrackLeafRoutePageView>
  );
};

export default HomePage;
