// Copyright 2023 The LUCI Authors.
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

import './analyses_page.css';

import { Search } from '@mui/icons-material';
import { Box, Button, TextField } from '@mui/material';
import { useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useTabId } from '@/generic_libs/components/routed_tabs';

import {
  ListTestAnalysesTable,
  SearchTestAnalysisTable,
} from '../components/analyses_tables';

const digitsRegex = new RegExp('^([0-9]*)$');

const isValidID = (id: string): boolean => {
  return digitsRegex.test(id);
};

export function TestAnalysesTab() {
  const [pendingSearchID, setPendingSearchID] = useState<string>('');
  const [searchID, setSearchID] = useState<string>('');
  const searchForAnalysis = () => {
    if (isValidID(pendingSearchID)) {
      setSearchID(pendingSearchID);
    }
  };
  return (
    <div className="analyses-tab">
      <Box
        justifyContent="center"
        sx={{
          paddingBottom: '1rem',
        }}
      >
        <TextField
          fullWidth
          size="small"
          variant="outlined"
          label="Analysis ID"
          onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
            const cleansedId = event.target.value.trim();
            setPendingSearchID(cleansedId);
          }}
          onKeyDown={(e) => {
            if (['Tab', 'Enter'].includes(e.key)) {
              searchForAnalysis();
            }
          }}
          error={!isValidID(pendingSearchID)}
          helperText={
            !isValidID(pendingSearchID)
              ? 'Invalid Analysis ID - enter digits only'
              : ''
          }
          InputProps={{
            endAdornment: (
              <Button
                color="primary"
                disabled={
                  !isValidID(pendingSearchID) || pendingSearchID === searchID
                }
                onClick={searchForAnalysis}
                startIcon={<Search />}
                sx={{
                  paddingLeft: '2rem',
                  paddingRight: '2rem',
                  borderRadius: 'none',
                }}
              >
                Search
              </Button>
            ),
          }}
          sx={{
            '& .MuiOutlinedInput-root': {
              paddingRight: '0',
            },
          }}
        />
      </Box>
      {searchID !== '' ? (
        <SearchTestAnalysisTable analysisId={searchID} />
      ) : (
        <ListTestAnalysesTable />
      )}
    </div>
  );
}

export function Component() {
  useTabId('test-analysis');

  return (
    <TrackLeafRoutePageView contentGroup="test-analysis">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="test-analysis"
      >
        <TestAnalysesTab />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
