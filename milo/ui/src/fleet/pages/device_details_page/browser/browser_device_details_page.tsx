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
import { useCallback, useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { PlatformNotAvailable } from '@/fleet/components/platform_not_available';
import {
  generateDeviceListURL,
  generateBrowserDeviceDetailsURL,
  CHROMIUM_PLATFORM,
} from '@/fleet/constants/paths';
import { usePlatform } from '@/fleet/hooks/usePlatform';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { BotData } from '../common/bot_data';
import { Tasks } from '../common/tasks_table';

import { BrowserDeviceDimensions } from './browser_device_dimensions';
import { InventoryData } from './inventory_data';
import { useBrowserDeviceData } from './use_browser_device_data';

enum TabValue {
  TASKS = 'tasks',
  BOT_INFO = 'bot-info',
  DIMENSIONS = 'dimensions',
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
      setSelectedTab(TabValue.DIMENSIONS);
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

// This page is only used in chrome os
export const BrowserDeviceDetailsPage = () => {
  const { id = '' } = useParams();
  const [deviceIdInputValue, setDeviceIdInputValue] = useState(id);
  const navigatedFromLink = useNavigatedFromLink();
  const [selectedTab, setSelectedTab] = useTabs();

  const navigate = useNavigate();
  const location = useLocation();
  const { error, isError, isLoading, device } = useBrowserDeviceData(id);
  const deviceIdInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setDeviceIdInputValue(id);
  }, [id]);

  useEffect(() => {
    const f = (e: KeyboardEvent) => {
      if (e.key === '/') {
        if (
          deviceIdInputRef.current &&
          !deviceIdInputRef.current.contains(document.activeElement)
        ) {
          e.preventDefault();
          deviceIdInputRef.current.focus();
        }
      }
    };

    window.addEventListener('keydown', f);
    return () => {
      window.removeEventListener('keydown', f);
    };
  }, []);

  const navigateToDeviceIfChanged = (deviceId: string) => {
    const parts = location.pathname.toString().split('/');
    const urlId = parts[parts.length - 1];
    if (urlId !== deviceId) {
      navigate(
        `${generateBrowserDeviceDetailsURL(deviceId)}?tab=${selectedTab}`,
      );
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
        <CentralizedProgress data-testid="loading-spinner" />
      </div>
    );
  }

  const swarmingInstance = device?.ufsLabels['swarming_instance']?.values?.[0];
  const swarmingHost = swarmingInstance && `${swarmingInstance}.appspot.com`;

  const botId = device?.ufsLabels['hostname']?.values?.at(0);

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
              navigate(generateDeviceListURL(CHROMIUM_PLATFORM));
            }
          }}
        >
          <ArrowBackIcon />
        </IconButton>
        <Typography variant="h4" sx={{ whiteSpace: 'nowrap' }}>
          Device details:
        </Typography>
        <TextField
          inputRef={deviceIdInputRef}
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
        {device === undefined && (
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
          ></div>
          <TabContext value={selectedTab || TabValue.TASKS}>
            <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
              <TabList onChange={(_, newValue) => setSelectedTab(newValue)}>
                <Tab label="Tasks" value={TabValue.TASKS} />
                <Tab label="Dimensions" value={TabValue.DIMENSIONS} />
                <Tab label="Inventory data" value={TabValue.INVENTORY_DATA} />
                <Tab label="Bot info" value={TabValue.BOT_INFO} />
              </TabList>
            </Box>
            <TabPanel value={TabValue.TASKS}>
              {swarmingInstance && botId ? (
                <Tasks botId={botId} swarmingHost={swarmingHost} />
              ) : (
                <UnavailableSwarmingInfo />
              )}
            </TabPanel>
            <TabPanel value={TabValue.DIMENSIONS}>
              <BrowserDeviceDimensions device={device} />
            </TabPanel>
            <TabPanel value={TabValue.INVENTORY_DATA}>
              <InventoryData device={device} />
            </TabPanel>
            <TabPanel value={TabValue.BOT_INFO}>
              {swarmingInstance && botId ? (
                <BotData botId={botId} swarmingHost={swarmingHost} />
              ) : (
                <UnavailableSwarmingInfo />
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
  const { platform } = usePlatform();
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-details">
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-device-details-page"
      >
        <FleetHelmet pageTitle={`${id ? `${id} | ` : ''}Device Details`} />
        <LoggedInBoundary>
          {platform !== Platform.CHROMIUM ? (
            <PlatformNotAvailable availablePlatforms={[Platform.CHROMIUM]} />
          ) : (
            <BrowserDeviceDetailsPage />
          )}
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}

const UnavailableSwarmingInfo = () => (
  <Alert severity="warning">
    Bot information not available (missing swarming instance or bot ID)
  </Alert>
);
