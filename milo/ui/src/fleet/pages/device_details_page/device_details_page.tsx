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
import {
  Alert,
  AlertTitle,
  Box,
  IconButton,
  Link,
  Typography,
} from '@mui/material';
import Tab from '@mui/material/Tab';
import { useCallback, useEffect, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { genFeedbackUrl } from '@/common/tools/utils';
import { RunAutorepair } from '@/fleet/components/actions/autorepair/run_autorepair';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { FEEDBACK_BUGANIZER_BUG_ID } from '@/fleet/constants/feedback';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { extractDutState, extractDutId } from '@/fleet/utils/devices';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { SchedulingData } from './scheduling_data_table';
import { Tasks } from './tasks_table';
import { useDeviceData } from './use_device_data';

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
  const { id = '' } = useParams();
  const navigatedFromLink = useNavigatedFromLink();
  const [selectedTab, setSelectedTab] = useTabs();

  const navigate = useNavigate();
  const { isLoading, device } = useDeviceData(id);

  if (isLoading) {
    return (
      <div
        css={{
          width: '100%',
          margin: '24px 0px',
        }}
      >
        <CentralizedProgress />
      </div>
    );
  }

  if (device === null) {
    return (
      <Alert severity="error">
        <AlertTitle>Device not found!</AlertTitle>
        <p>
          Oh no! The device <code>{id}</code> you are looking for was not found.
        </p>
        <p>
          If you believe that the device should be there, let us know by
          submitting your{' '}
          <Link
            href={genFeedbackUrl({
              bugComponent: FEEDBACK_BUGANIZER_BUG_ID,
              errMsg: `Device not found: ${id}`,
            })}
            target="_blank"
          >
            feedback
          </Link>
          !
        </p>
      </Alert>
    );
  }

  const dutId = extractDutId(device);

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
      <div
        css={{
          boxSizing: 'border-box',
          width: '100%',
          paddingLeft: '72px',
          marginBottom: '32px',
        }}
      >
        <RunAutorepair
          selectedDuts={[
            {
              name: id,
              state: extractDutState(device),
            },
          ]}
        />
      </div>

      <TabContext value={selectedTab || TabValue.TASKS}>
        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <TabList onChange={(_, newValue) => setSelectedTab(newValue)}>
            <Tab label="Tasks" value={TabValue.TASKS} />
            <Tab label="Scheduling labels" value={TabValue.SCHEDULING} />
          </TabList>
        </Box>
        <TabPanel value={TabValue.TASKS}>
          <Tasks dutId={dutId} />
        </TabPanel>
        <TabPanel value={TabValue.SCHEDULING}>
          <SchedulingData device={device} />
        </TabPanel>
      </TabContext>
    </div>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-details">
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-device-details-page"
      >
        {/** TODO: Update this to show the ID of the device being viewed.  */}
        <FleetHelmet pageTitle="Device Details" />
        <LoggedInBoundary>
          <DeviceDetailsPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
