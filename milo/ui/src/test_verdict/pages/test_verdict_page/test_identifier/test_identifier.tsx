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

import CancelIcon from '@mui/icons-material/Cancel';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import NextPlanIcon from '@mui/icons-material/NextPlan';
import NotStartedIcon from '@mui/icons-material/NotStarted';
import QuestionMarkIcon from '@mui/icons-material/QuestionMark';
import RemoveCircleIcon from '@mui/icons-material/RemoveCircle';
import ReportIcon from '@mui/icons-material/Report';
import WarningIcon from '@mui/icons-material/Warning';
import Grid from '@mui/material/Grid2';
import { upperFirst } from 'lodash-es';

import {
  VERDICT_STATUS_DISPLAY_MAP,
  VERDICT_STATUS_OVERRIDE_DISPLAY_MAP,
} from '@/common/constants/verdict';
import {
  SpecifiedTestVerdictStatusOverride,
  SpecifiedTestVerdictStatus,
} from '@/common/types/verdict';
import {
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

import { useTestVerdict } from '../context';

import { CLInfo } from './cl_info';

function getTestVariantStatusLabel(
  status: SpecifiedTestVerdictStatus,
  statusOverride: SpecifiedTestVerdictStatusOverride,
) {
  if (statusOverride !== TestVerdict_StatusOverride.NOT_OVERRIDDEN) {
    return upperFirst(VERDICT_STATUS_OVERRIDE_DISPLAY_MAP[statusOverride]);
  }
  return upperFirst(VERDICT_STATUS_DISPLAY_MAP[status]);
}

function getTestVariantStatusIcon(
  status: SpecifiedTestVerdictStatus,
  statusOverride: SpecifiedTestVerdictStatusOverride,
) {
  if (statusOverride !== TestVerdict_StatusOverride.NOT_OVERRIDDEN) {
    switch (statusOverride) {
      case TestVerdict_StatusOverride.EXONERATED:
        return <RemoveCircleIcon className="exonerated-verdict" />;
      default:
        return <QuestionMarkIcon className="unspecified" />;
    }
  }

  switch (status) {
    case TestVerdict_Status.FAILED:
      return <CancelIcon className="failed-verdict" />;
    case TestVerdict_Status.EXECUTION_ERRORED:
      return <ReportIcon className="execution-errored-verdict" />;
    case TestVerdict_Status.PRECLUDED:
      return <NotStartedIcon className="precluded-verdict" />;
    case TestVerdict_Status.FLAKY:
      return <WarningIcon className="flaky-verdict" />;
    case TestVerdict_Status.PASSED:
      return <CheckCircleIcon className="passed-verdict" />;
    case TestVerdict_Status.SKIPPED:
      return <NextPlanIcon className="skipped-verdict" />;
    default:
      return <QuestionMarkIcon className="unspecified" />;
  }
}

export function TestIdentifier() {
  const { statusV2, statusOverride, testId } = useTestVerdict();

  return (
    <Grid container rowGap={1}>
      <CLInfo />
      <Grid
        container
        columnGap={1}
        alignItems="center"
        sx={{
          fontSize: '1.2rem',
          fontWeight: '700',
          mb: 1,
        }}
      >
        {getTestVariantStatusIcon(statusV2, statusOverride)}
        {getTestVariantStatusLabel(statusV2, statusOverride)}: {testId}
      </Grid>
    </Grid>
  );
}
