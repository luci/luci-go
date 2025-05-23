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

import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import ErrorIcon from '@mui/icons-material/Error';
import NextPlanIcon from '@mui/icons-material/NextPlan';
import NotStartedIcon from '@mui/icons-material/NotStarted';
import QuestionMarkIcon from '@mui/icons-material/QuestionMark';
import ReportIcon from '@mui/icons-material/Report';
import Grid from '@mui/material/Grid2';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import Tooltip from '@mui/material/Tooltip';
import { useEffectOnce } from 'react-use';

import {
  TEST_STATUS_V2_CLASS_MAP,
  TEST_STATUS_V2_DISPLAY_MAP,
} from '@/common/constants/test';
import { setSingleQueryParam } from '@/common/tools/url_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  TestResult,
  TestResult_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { getSuggestedResultId } from '@/test_verdict/tools/test_result_utils';

import { useResults } from '../context';
import { RESULT_ID_SEARCH_PARAM_KEY, getSelectedResultId } from '../utils';

function getRunStatusIcon(status: TestResult_Status) {
  const className = TEST_STATUS_V2_CLASS_MAP[status];
  switch (status) {
    case TestResult_Status.FAILED:
      return <ErrorIcon className={className} />;
    case TestResult_Status.EXECUTION_ERRORED:
      return <ReportIcon className={className} />;
    case TestResult_Status.PRECLUDED:
      return <NotStartedIcon className={className} />;
    case TestResult_Status.PASSED:
      return <CheckCircleIcon className={className} />;
    case TestResult_Status.SKIPPED:
      return <NextPlanIcon className={className} />;
    default:
      return <QuestionMarkIcon className={className} />;
  }
}

function getTitle(result: TestResult) {
  return TEST_STATUS_V2_DISPLAY_MAP[result.statusV2];
}

export function ResultsHeader() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const selectedResultId = getSelectedResultId(searchParams);
  const results = useResults();

  useEffectOnce(() => {
    if (selectedResultId === null) {
      updateSelectedTabIndex(getSuggestedResultId(results));
    }
  });

  function updateSelectedTabIndex(id: string) {
    setSearchParams(
      setSingleQueryParam(
        searchParams.toString(),
        RESULT_ID_SEARCH_PARAM_KEY,
        id,
      ),
    );
  }

  if (!selectedResultId) {
    return <></>;
  }
  return (
    <Grid
      sx={{ borderBottom: 1, borderColor: 'divider' }}
      alignItems="center"
      flexGrow="1"
    >
      <Tabs
        value={selectedResultId}
        aria-label="Test verdict results"
        sx={{
          minHeight: '30px',
        }}
        scrollButtons="auto"
        onChange={(_, newValue: string) => updateSelectedTabIndex(newValue)}
      >
        {results.map((result, i) => (
          <Tab
            key={result.result.resultId}
            value={result.result.resultId}
            sx={{
              minHeight: 0,
            }}
            icon={
              <Tooltip title={getTitle(result.result)}>
                {getRunStatusIcon(result.result.statusV2)}
              </Tooltip>
            }
            iconPosition="end"
            label={`Result ${i + 1}`}
          />
        ))}
      </Tabs>
    </Grid>
  );
}
