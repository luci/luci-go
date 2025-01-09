// Copyright 2024 The LUCI Authors.
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

import TabContext from '@mui/lab/TabContext';
import TabList from '@mui/lab/TabList';
import TabPanel from '@mui/lab/TabPanel';
import { Box, Container, Paper } from '@mui/material';
import Grid from '@mui/material/Grid2';
import Tab from '@mui/material/Tab';
import { Helmet } from 'react-helmet';
import { useParams } from 'react-router-dom';

import GridLabel from '@/clusters/components/grid_label/grid_label';
import PanelHeading from '@/clusters/components/headings/panel_heading/panel_heading';
import bassFavicon from '@/common/assets/favicons/bass-32.png';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

interface Device {
  id: string;
}

interface DeviceDetailsHeaderProps {
  device: Device;
}

export const DeviceDetailsHeader = ({ device }: DeviceDetailsHeaderProps) => {
  return (
    <Paper
      data-cy="device-details-header"
      elevation={3}
      sx={{ pt: 2, pb: 2, mt: 1, mx: 3 }}
    >
      <Container maxWidth={false}>
        <PanelHeading>Device details: __DEVICE_NAME__</PanelHeading>
        <Grid container rowGap={1}>
          <GridLabel text="Device ID" />
          <Grid alignItems="center" columnGap={1} size={10}>
            <Box
              sx={{ display: 'inline-block' }}
              paddingTop={1}
              paddingRight={1}
            >
              {device.id}
            </Box>
          </Grid>
          <GridLabel text="Param1" />
          <Grid alignItems="center" columnGap={1} size={10}>
            <Box
              sx={{ display: 'inline-block' }}
              paddingTop={1}
              paddingRight={1}
            >
              __VALUE 1__
            </Box>
          </Grid>
          <GridLabel text="Param2" />
          <Grid alignItems="center" columnGap={1} size={10}>
            <Box
              sx={{ display: 'inline-block' }}
              paddingTop={1}
              paddingRight={1}
            >
              __VALUE 2__
            </Box>
          </Grid>
        </Grid>
      </Container>
    </Paper>
  );
};

export const DeviceDetails = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const handleTabChange = (newValue: string) => {
    setSearchParams(
      (params) => {
        params.set('tab', newValue);
        return params;
      },
      { replace: true },
    );
  };

  const validValues = ['tasks', 'events', 'scheduling'];
  let selectedTab = searchParams.get('tab');
  if (!selectedTab || validValues.indexOf(selectedTab) === -1) {
    selectedTab = 'overview';
  }

  return (
    <Paper
      data-cy="device-details"
      elevation={3}
      sx={{ pt: 2, pb: 2, mt: 3, mx: 3 }}
    >
      <Container maxWidth={false}>
        <TabContext value={selectedTab}>
          <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
            <TabList onChange={(_, newValue) => handleTabChange(newValue)}>
              <Tab label="Tasks" value="tasks" />
              <Tab label="Events" value="events" />
              <Tab label="Scheduling data" value="scheduling" />
            </TabList>
          </Box>
          <TabPanel value="tasks">
            <span>__TASKS__</span>
          </TabPanel>
          <TabPanel value="events">
            <span>__EVENTS__</span>
          </TabPanel>
          <TabPanel value="scheduling">
            <span>__SCHEDULING DATA__</span>
          </TabPanel>
        </TabContext>
      </Container>
    </Paper>
  );
};

export const DeviceDetailsPage = () => {
  const { id } = useParams();

  const device: Device = {
    id: id || '',
  };

  return (
    <>
      <DeviceDetailsHeader device={device} />
      <DeviceDetails />
    </>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-list">
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-device-details-page"
      >
        <Helmet>
          <title>Streamlined Fleet UI</title>
          <link rel="icon" type="image/x-icon" href={bassFavicon} />
        </Helmet>
        <DeviceDetailsPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
