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

import { TEST_STATUS_DISPLAY_MAP } from '@/common/constants';
import { TestResult, TestStatus } from '@/common/services/resultdb';

import {
  useResults,
  useSelectedResultIndex,
  useSetSelectedResultIndex,
} from '../context';

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

// TODO(b/308716499): Make tabs navigatable.
export function ResultsHeader() {
  const selectedResult = useSelectedResultIndex();
  const setSelectedResult = useSetSelectedResultIndex();
  const results = useResults();
  return (
    <Grid
      item
      sx={{ borderBottom: 1, borderColor: 'divider' }}
      alignItems="center"
      flexGrow="1"
    >
      <Tabs
        value={selectedResult}
        aria-label="Test verdict runs"
        sx={{
          minHeight: '30px',
        }}
        onChange={(_, newValue: number) => setSelectedResult(newValue)}
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
