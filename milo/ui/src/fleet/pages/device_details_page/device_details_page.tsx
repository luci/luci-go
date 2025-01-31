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

import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import TabContext from '@mui/lab/TabContext';
import TabList from '@mui/lab/TabList';
import TabPanel from '@mui/lab/TabPanel';
import { Box, IconButton, Typography } from '@mui/material';
import Tab from '@mui/material/Tab';
import { useCallback, useEffect, useState } from 'react';
import { Helmet } from 'react-helmet';
import { useLocation, useNavigate, useParams } from 'react-router-dom';

import bassFavicon from '@/common/assets/favicons/bass-32.png';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { SchedulingData } from './scheduling_data_table';
import { Tasks } from './tasks_table';

enum TabValue {
  TASKS = 'tasks',
  SCHEDULING = 'scheduling',
}

const parseTabValue = (tabString: string | null): TabValue | undefined => {
  if (!tabString) {
    return undefined;
  }
  const val = tabString as TabValue;
  if (Object.values(TabValue).includes(val)) {
    return val;
  }
  return undefined;
};

const useTabs = (): [TabValue | undefined, (newValue: TabValue) => void] => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const setSelectedTab = useCallback(
    (newValue: TabValue) => {
      setSearchParams(
        (params) => {
          params.set('tab', newValue);
          return params;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const selectedTab = parseTabValue(searchParams.get('tab'));

  useEffect(() => {
    if (!selectedTab) {
      setSelectedTab(TabValue.TASKS);
    }
  }, [selectedTab, setSelectedTab]);

  return [selectedTab, setSelectedTab];
};

const useNavigatedFromLink = () => {
  const { state } = useLocation();

  const [navigatedFromLink, setNavigatedFromLink] = useState<
    string | undefined
  >(undefined);

  // state from useLocation is lost on rerenders so we need to save it in a react state
  useEffect(() => {
    if (state) {
      setNavigatedFromLink(state.navigatedFromLink);
    }
  }, [state]);

  return navigatedFromLink;
};

export const DeviceDetailsPage = () => {
  const { id } = useParams();
  const navigatedFromLink = useNavigatedFromLink();
  const [selectedTab, setSelectedTab] = useTabs();

  const navigate = useNavigate();

  if (!id || !selectedTab) {
    return <></>;
  }

  return (
    <div
      css={{
        margin: '24px 0px',
      }}
    >
      <div css={{ width: '100%' }}>
        <div
          css={{
            margin: '0px 24px 16px 24px',
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            gap: 10,
          }}
        >
          <IconButton
            onClick={() => {
              if (navigatedFromLink) {
                navigate(navigatedFromLink);
              } else {
                navigate('/ui/fleet/labs/devices');
              }
            }}
          >
            <ArrowBackIcon />
          </IconButton>
          <Typography variant="h4">Device details: {id}</Typography>
        </div>
        <TabContext value={selectedTab}>
          <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
            <TabList onChange={(_, newValue) => setSelectedTab(newValue)}>
              <Tab label="Tasks" value={TabValue.TASKS} />
              <Tab label="Scheduling data" value={TabValue.SCHEDULING} />
            </TabList>
          </Box>
          <TabPanel value={TabValue.TASKS}>
            <Tasks />
          </TabPanel>
          <TabPanel value={TabValue.SCHEDULING}>
            <SchedulingData id={id} />
          </TabPanel>
        </TabContext>
      </div>
    </div>
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
        <LoggedInBoundary>
          <DeviceDetailsPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
