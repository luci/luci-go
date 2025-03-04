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
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import FormControl from '@mui/material/FormControl';
import IconButton from '@mui/material/IconButton';
import InputAdornment from '@mui/material/InputAdornment';
import Paper from '@mui/material/Paper';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useState } from 'react';
import { Helmet } from 'react-helmet';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useDeclarePageId } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import { LookupResults } from '../components/lookup_results';
export function LookupPage() {
  const [query, setQuery] = useState<string>('');
  const [principal, setPrincipal] = useState<string>('');

  const searchQuery = () => {
    setPrincipal(query);
  };
  return (
    <Paper className="lookup-container-paper">
      <Alert severity="warning">
        <AlertTitle>
          Integration of LUCI Auth Service here is under construction.
        </AlertTitle>
        Only group editing is supported. Please visit{' '}
        <a
          href={`https://${SETTINGS.authService.host}/auth/groups`}
          target="_blank"
          rel="noreferrer"
        >
          Auth Service
        </a>{' '}
        for other functionality. Please provide{' '}
        <a
          href="https://b.corp.google.com/issues/new?component=1435307&template=2026255"
          target="_blank"
          rel="noreferrer"
        >
          feedback.
        </a>
      </Alert>
      <Box sx={{ p: 5 }}>
        <Typography variant="body2">
          Look up a principal (user, glob or group) to find its ancestors
          (groups the principal is included by).
        </Typography>
        <FormControl fullWidth>
          <TextField
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            label="An email, glob or group name. E.g. person@example.com, *@google.com, administrators."
            slotProps={{
              input: {
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton onClick={searchQuery}>
                      <SearchIcon />
                    </IconButton>
                  </InputAdornment>
                ),
              },
            }}
          ></TextField>
        </FormControl>
        <LookupResults name={principal || ''} />
      </Box>
    </Paper>
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
