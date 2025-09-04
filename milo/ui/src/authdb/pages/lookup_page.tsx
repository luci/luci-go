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

import '../components/groups.css';
import SearchIcon from '@mui/icons-material/Search';
import TabContext from '@mui/lab/TabContext';
import TabList from '@mui/lab/TabList';
import TabPanel from '@mui/lab/TabPanel';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import FormControl from '@mui/material/FormControl';
import IconButton from '@mui/material/IconButton';
import InputAdornment from '@mui/material/InputAdornment';
import Paper from '@mui/material/Paper';
import Tab from '@mui/material/Tab';
import TextField from '@mui/material/TextField';
import { createTheme, ThemeProvider } from '@mui/material/styles';
import { useState } from 'react';
import { Helmet } from 'react-helmet';

import { LookupResults } from '@/authdb/components/lookup_results';
import { PermissionsResults } from '@/authdb/components/permissions_results';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useDeclarePageId } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

const PRINCIPAL_PARAM_KEY = 'p';
const TAB_PARAM_KEY = 'tab';

const theme = createTheme({
  typography: {
    h6: {
      color: 'black',
      margin: '0',
      padding: '0',
      fontSize: '1.17em',
      fontWeight: 'bold',
    },
  },
  components: {
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderBottom: 'none',
          paddingLeft: '0',
          paddingBottom: '5px',
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          paddingTop: '0',
        },
      },
    },
  },
});

export function LookupPage() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const principal = searchParams.get(PRINCIPAL_PARAM_KEY);
  const [barInput, setBarInput] = useState(principal || '');

  const searchQuery = () => {
    setSearchParams(
      (params) => {
        params.set(PRINCIPAL_PARAM_KEY, barInput);
        return params;
      },
      { replace: true },
    );
  };

  const checkFieldSubmit = (keyPressed: string) => {
    if (keyPressed !== 'Enter') {
      return;
    }
    searchQuery();
  };

  const handleTabChange = (newValue: string) => {
    setSearchParams(
      (params) => {
        params.set(TAB_PARAM_KEY, newValue);
        return params;
      },
      { replace: true },
    );
  };

  const validValues = ['ancestors', 'permissions'];
  let tabValue = searchParams.get(TAB_PARAM_KEY);
  if (!tabValue || validValues.indexOf(tabValue) === -1) {
    // Default to the ancestors tab.
    tabValue = 'ancestors';
  }

  function a11yProps(value: string) {
    return {
      id: `tab-${value}`,
      'aria-controls': `tabpanel-${value}`,
      value: `${value}`,
    };
  }

  return (
    <ThemeProvider theme={theme}>
      <Paper className="lookup-container-paper">
        <Alert severity="info">
          <AlertTitle> You are using the new UI for Auth Service.</AlertTitle>
          If there is additional functionality you would like supported, file a
          bug using the{' '}
          <a
            href="https://b.corp.google.com/issues/new?component=1435307&template=2026255"
            target="_blank"
            rel="noreferrer"
          >
            feedback link
          </a>{' '}
          and use the{' '}
          <a
            href={`https://${SETTINGS.authService.host}/auth/lookup`}
            target="_blank"
            rel="noreferrer"
          >
            previous UI
          </a>{' '}
          in the meantime.
        </Alert>
        <Box sx={{ p: 5 }}>
          <FormControl fullWidth>
            <TextField
              data-testid="lookup-textfield"
              value={barInput}
              onChange={(e) => setBarInput(e.target.value)}
              onKeyDown={(e) => checkFieldSubmit(e.key)}
              label="Look up an email, glob, or group name. e.g. person@example.com, *@google.com, administrators."
              slotProps={{
                input: {
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        onClick={searchQuery}
                        data-testid="search-button"
                      >
                        <SearchIcon />
                      </IconButton>
                    </InputAdornment>
                  ),
                },
              }}
            ></TextField>
          </FormControl>
          {principal && (
            <TabContext value={tabValue}>
              <Box sx={{ borderBottom: 1, borderColor: 'divider', pt: 1.5 }}>
                <TabList onChange={(_, newValue) => handleTabChange(newValue)}>
                  <Tab label="Ancestors" {...a11yProps('ancestors')} />
                  <Tab
                    label="Permissions"
                    {...a11yProps('permissions')}
                    data-testid="permissions-tab"
                  />
                </TabList>
              </Box>
              <TabPanel
                value="ancestors"
                role="tabpanel"
                id="tabpanel-ancestors"
              >
                <LookupResults name={principal || ''} />
              </TabPanel>
              <TabPanel
                value="permissions"
                role="tabpanel"
                id="tabpanel-permissions"
              >
                <PermissionsResults principal={principal || ''} />
              </TabPanel>
            </TabContext>
          )}
        </Box>
      </Paper>
    </ThemeProvider>
  );
}

export function Component() {
  useDeclarePageId(UiPage.AuthServiceLookup);

  return (
    <TrackLeafRoutePageView contentGroup="authdb-lookup">
      <Helmet>
        <title>Lookup user</title>
      </Helmet>
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="authdb-lookup"
      >
        <LookupPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
