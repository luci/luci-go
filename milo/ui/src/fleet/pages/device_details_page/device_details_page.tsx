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
import { Alert, Box, IconButton, TextField, Typography } from '@mui/material';
import Tab from '@mui/material/Tab';
import { useCallback, useEffect, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { RunAutorepair } from '@/fleet/components/actions/autorepair/run_autorepair';
import { SshTip } from '@/fleet/components/actions/ssh/ssh_tip';
import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { extractDutState, extractDutId } from '@/fleet/utils/devices';
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { BotData } from './bot_data';
import { InventoryData } from './inventory_data';
import { SchedulingData } from './scheduling_data_table';
import { Tasks } from './tasks_table';
import { useDeviceData } from './use_device_data';

enum TabValue {
  TASKS = 'tasks',
  BOT_INFO = 'bot-info',
  SCHEDULING = 'scheduling',
  INVENTORY_DATA = 'inventory',
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
      setSelectedTab(TabValue.SCHEDULING);
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
  const [deviceIdInputValue, setDeviceIdInputValue] = useState(id);
  const navigatedFromLink = useNavigatedFromLink();
  const [selectedTab, setSelectedTab] = useTabs();

  const navigate = useNavigate();
  const location = useLocation();
  const { error, isError, isLoading, device } = useDeviceData(id);

  useEffect(() => {
    setDeviceIdInputValue(id);
  }, [id]);

  const navigateToDeviceIfChanged = (deviceId: string) => {
    const parts = location.pathname.toString().split('/');
    const urlId = parts[parts.length - 1];
    if (urlId !== deviceId) {
      // TODO: b/402770033 - fix URL generation
      navigate(`/ui/fleet/labs/devices/${deviceId}?tab=${selectedTab}`);
    }
  };

  if (isError) {
    return (
      <Alert severity="error">
        Something went wrong: {getErrorMessage(error, 'fetch device')}
      </Alert>
    );
  }

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
        <Typography variant="h4" sx={{ whiteSpace: 'nowrap' }}>
          Device details:
        </Typography>
        <TextField
          variant="standard"
          value={deviceIdInputValue}
          onChange={(event) => setDeviceIdInputValue(event.target.value)}
          slotProps={{ htmlInput: { sx: { fontSize: 24 } } }}
          fullWidth
          onBlur={(e) => {
            navigateToDeviceIfChanged(e.target.value);
          }}
          onKeyDown={(e) => {
            const target = e.target as HTMLInputElement;
            if (e.key === 'Enter') {
              navigateToDeviceIfChanged(target.value);
            }
          }}
        />
      </div>
      <>
        {device === null && (
          <AlertWithFeedback
            title="Device not found!"
            bugErrorMessage={`Device not found: ${id}`}
          >
            <p>
              Oh no! The device <code>{id}</code> you are looking for was not
              found.
            </p>
          </AlertWithFeedback>
        )}
      </>
      {device && (
        <>
          <div
            css={{
              boxSizing: 'border-box',
              width: '100%',
              paddingLeft: '72px',
              marginBottom: '32px',
              display: 'flex',
              flexDirection: 'row',
              gap: '8px',
            }}
          >
            <RunAutorepair
              selectedDuts={[
                {
                  name: id,
                  // Confusingly, dutID, which is the asset tag of the DUT is
                  // different from "id", which is the internal ID used within
                  // the Fleet Console. For ChromeOS DUTs, the Fleet Console
                  // populates the "ID" for a DUT using the DUT's hostname.
                  dutId,
                  state: extractDutState(device),
                },
              ]}
            />
            <SshTip deviceId={id} />
          </div>
          <TabContext value={selectedTab || TabValue.TASKS}>
            <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
              <TabList onChange={(_, newValue) => setSelectedTab(newValue)}>
                <Tab label="Tasks" value={TabValue.TASKS} />
                <Tab label="Scheduling labels" value={TabValue.SCHEDULING} />
                <Tab label="Inventory data" value={TabValue.INVENTORY_DATA} />
                <Tab label="Bot info" value={TabValue.BOT_INFO} />
              </TabList>
            </Box>
            <TabPanel value={TabValue.TASKS}>
              {dutId === '' ? (
                <Alert severity="warning">
                  No <code>dutID</code> set on this device
                </Alert>
              ) : (
                <Tasks dutId={dutId} />
              )}
            </TabPanel>
            <TabPanel value={TabValue.INVENTORY_DATA}>
              <InventoryData hostname={id} />
            </TabPanel>
            <TabPanel value={TabValue.SCHEDULING}>
              <SchedulingData device={device} />
            </TabPanel>
            <TabPanel value={TabValue.BOT_INFO}>
              {dutId === '' ? (
                <Alert severity="warning">
                  No <code>dutID</code> set on this device
                </Alert>
              ) : (
                <BotData dutId={dutId} />
              )}
            </TabPanel>
          </TabContext>
        </>
      )}
    </div>
  );
};

export function Component() {
  const { id = '' } = useParams();
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-details">
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-device-details-page"
      >
        <FleetHelmet pageTitle={`${id ? `${id} | ` : ''}Device Details`} />
        <LoggedInBoundary>
          <DeviceDetailsPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
