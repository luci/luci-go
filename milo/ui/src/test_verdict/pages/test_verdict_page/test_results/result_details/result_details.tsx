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
import Grid from '@mui/material/Grid';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { useResults } from '../hooks';
import { getSelectedResultIndex } from '../utils';

import { ResultDataProvider } from './context';
import { useSetTopPanelExpanded, useTopPanelExpanded } from './hooks';
import { ResultArtifacts } from './result_artifacts';
import { ResultBasicInfo } from './result_basic_info';
import { ResultLogs } from './result_logs';
import { ResultTags } from './result_tags';

function TopPanel() {
  const topPanelExpanded = useTopPanelExpanded();
  const setTopPanelExpandendCtx = useSetTopPanelExpanded();

  function handleToggleTopPanel() {
    setTopPanelExpandendCtx(!topPanelExpanded);
  }

  return (
    <>
      <Collapse in={topPanelExpanded}>
        <Grid
          item
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
  const selecteResultIndex = getSelectedResultIndex(searchParams);

  if (selecteResultIndex === null) {
    // This component should not fail if there is no selected result
    // as the default result will be selected auomatically,
    // but it also should not render anything as that would increase load time.
    return <></>;
  }

  const result = results[selecteResultIndex];

  if (!result) {
    throw new Error('Selected result index out of bounds.');
  }

  return (
    <ResultDataProvider result={result.result}>
      <TopPanel />
      <ResultLogs />
    </ResultDataProvider>
  );
}
