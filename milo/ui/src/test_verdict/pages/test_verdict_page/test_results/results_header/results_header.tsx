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

import BrokenImageIcon from '@mui/icons-material/BrokenImage';
import CancelIcon from '@mui/icons-material/Cancel';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import DoNotDisturbIcon from '@mui/icons-material/DoNotDisturb';
import DoNotDisturbOnTotalSilenceIcon from '@mui/icons-material/DoNotDisturbOnTotalSilence';
import QuestionMarkIcon from '@mui/icons-material/QuestionMark';
import Grid from '@mui/material/Grid';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import Tooltip from '@mui/material/Tooltip';
import { useEffectOnce } from 'react-use';

import { TEST_STATUS_DISPLAY_MAP } from '@/common/constants/test';
import { setSingleQueryParam } from '@/common/tools/url_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  TestResult,
  TestStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { getSuggestedResultIndex } from '@/test_verdict/tools/test_result_utils';

import { useResults } from '../hooks';
import {
  RESULT_INDEX_SEARCH_PARAM_KEY,
  getSelectedResultIndex,
} from '../utils';

function getRunStatusIcon(status: TestStatus, expected: boolean) {
  const className = expected ? 'expected' : 'unexpected';
  switch (status) {
    case TestStatus.ABORT:
      return <DoNotDisturbOnTotalSilenceIcon className={className} />;
    case TestStatus.CRASH:
      return <BrokenImageIcon className={className} />;
    case TestStatus.FAIL:
      return <CancelIcon className={className} />;
    case TestStatus.PASS:
      return <CheckCircleIcon className={className} />;
    case TestStatus.SKIP:
      return <DoNotDisturbIcon className={className} />;
    case TestStatus.STATUS_UNSPECIFIED:
    default:
      return <QuestionMarkIcon className={className} />;
  }
}

function getTitle(result: TestResult) {
  return `${result.expected ? 'Expectedly' : 'Unexpectedly'} ${
    TEST_STATUS_DISPLAY_MAP[result.status]
  }`;
}

export function ResultsHeader() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const selectedResultIndex = getSelectedResultIndex(searchParams);
  const results = useResults();

  useEffectOnce(() => {
    if (selectedResultIndex === null) {
      updateSelectedTabIndex(getSuggestedResultIndex(results));
    }
  });

  function updateSelectedTabIndex(index: number) {
    setSearchParams(
      setSingleQueryParam(
        searchParams.toString(),
        RESULT_INDEX_SEARCH_PARAM_KEY,
        index.toString(),
      ),
    );
  }

  return (
    <Grid
      item
      sx={{ borderBottom: 1, borderColor: 'divider' }}
      alignItems="center"
      flexGrow="1"
    >
      <Tabs
        value={selectedResultIndex || 0}
        aria-label="Test verdict results"
        sx={{
          minHeight: '30px',
        }}
        onChange={(_, newValue: number) => updateSelectedTabIndex(newValue)}
      >
        {results.map((result, i) => (
          <Tooltip title={getTitle(result.result)} key={result.result.resultId}>
            <Tab
              sx={{
                minHeight: 0,
              }}
              icon={getRunStatusIcon(
                result.result.status,
                result.result.expected,
              )}
              iconPosition="end"
              label={`Result ${i + 1}`}
            />
          </Tooltip>
        ))}
      </Tabs>
    </Grid>
  );
}
