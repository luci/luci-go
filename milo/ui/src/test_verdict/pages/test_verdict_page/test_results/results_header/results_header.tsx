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
import { useEffectOnce } from 'react-use';

import { TEST_STATUS_DISPLAY_MAP } from '@/common/constants';
import {
  TestResult,
  TestResultBundle,
  TestStatus,
} from '@/common/services/resultdb';
import { setSingleQueryParam } from '@/common/tools/url_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { useResults } from '../context';
import {
  RESULT_INDEX_SEARCH_PARAM_KEY,
  getSelectedResultIndex,
} from '../utils';

function getRunStatusIcon(status: TestStatus) {
  switch (status) {
    case TestStatus.Abort:
      return <DoNotDisturbOnTotalSilenceIcon className="exonerated" />;
    case TestStatus.Crash:
      return <BrokenImageIcon className="unexpected" />;
    case TestStatus.Fail:
      return <CancelIcon className="unexpected" />;
    case TestStatus.Pass:
      return <CheckCircleIcon className="expected" />;
    case TestStatus.Skip:
      return <DoNotDisturbIcon className="unexpectedly-skipped" />;
    case TestStatus.Unspecified:
    default:
      return <QuestionMarkIcon className="unspecified" />;
  }
}

function getTitle(result: TestResult) {
  return `${result.expected ? 'Expectedly' : 'Unexpectedly'} ${
    TEST_STATUS_DISPLAY_MAP[result.status]
  }`;
}

function getFirstFailedResult(results: readonly TestResultBundle[]) {
  return results.findIndex((e) => !e.result?.expected) ?? 0;
}

export function ResultsHeader() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const selectedResultIndex = getSelectedResultIndex(searchParams);
  const results = useResults();

  useEffectOnce(() => {
    if (selectedResultIndex === null) {
      updateSelectedTabIndex(getFirstFailedResult(results));
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
        aria-label="Test verdict runs"
        sx={{
          minHeight: '30px',
        }}
        onChange={(_, newValue: number) => updateSelectedTabIndex(newValue)}
      >
        {results.map((result, i) => (
          <Tab
            sx={{
              minHeight: 0,
            }}
            key={result.result.resultId}
            icon={getRunStatusIcon(result.result.status)}
            iconPosition="end"
            title={getTitle(result.result)}
            label={`Run ${i + 1}`}
          />
        ))}
      </Tabs>
    </Grid>
  );
}
