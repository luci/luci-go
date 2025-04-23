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

import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { Collapse, Divider, IconButton } from '@mui/material';
import Grid from '@mui/material/Grid2';
import { useEffect } from 'react';

import {
  searchParamUpdater,
  useSyncedSearchParams,
} from '@/generic_libs/hooks/synced_search_params';

import { useResults } from '../context';
import { getSelectedResultId } from '../utils';

import {
  TOP_PANEL_EXPANDED_OFF,
  TOP_PANEL_EXPANDED_ON,
  TOP_PANEL_EXPANDED_PARAM,
} from './constants';
import { ResultDataProvider } from './context';
import { ResultArtifacts } from './result_artifacts';
import { ResultBasicInfo } from './result_basic_info';
import { ResultLogs } from './result_logs';
import { ResultTags } from './result_tags';

function TopPanel() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const topPanelExpandedParam = searchParams.get(TOP_PANEL_EXPANDED_PARAM);

  useEffect(() => {
    if (topPanelExpandedParam === null) {
      setSearchParams(
        searchParamUpdater(TOP_PANEL_EXPANDED_PARAM, TOP_PANEL_EXPANDED_ON),
      );
    }
  }, [setSearchParams, topPanelExpandedParam]);

  function handleToggleTopPanel() {
    const isOpen =
      topPanelExpandedParam === TOP_PANEL_EXPANDED_ON
        ? TOP_PANEL_EXPANDED_OFF
        : TOP_PANEL_EXPANDED_ON;
    setSearchParams(searchParamUpdater(TOP_PANEL_EXPANDED_PARAM, isOpen));
  }

  const topPanelExpanded = topPanelExpandedParam === '1';

  return (
    <>
      <Collapse in={topPanelExpanded}>
        <Grid
          container
          flexDirection="column"
          sx={{
            width: '100%',
          }}
        >
          <ResultBasicInfo />
          <ResultTags />
          <ResultArtifacts />
        </Grid>
      </Collapse>
      <Divider
        sx={{
          padding: 0,
          mt: 1,
        }}
      >
        <IconButton size="small" onClick={handleToggleTopPanel}>
          {topPanelExpanded ? (
            <ExpandLessIcon fontSize="small" />
          ) : (
            <ExpandMoreIcon fontSize="small" />
          )}
        </IconButton>
      </Divider>
    </>
  );
}

export function ResultDetails() {
  const [searchParams] = useSyncedSearchParams();
  const results = useResults();
  const selecteResultId = getSelectedResultId(searchParams);

  if (selecteResultId === null) {
    // This component should not fail if there is no selected result
    // as the default result will be selected auomatically,
    // but it also should not render anything as that would increase load time.
    return <></>;
  }

  const result = results.find((r) => r.result.resultId === selecteResultId);

  if (!result) {
    throw new Error('Selected result id is required.');
  }

  return (
    <ResultDataProvider result={result.result}>
      <TopPanel />
      <ResultLogs />
    </ResultDataProvider>
  );
}
