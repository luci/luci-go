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
import QuestionMarkIcon from '@mui/icons-material/QuestionMark';
import RemoveCircleIcon from '@mui/icons-material/RemoveCircle';
import ReportIcon from '@mui/icons-material/Report';
import WarningIcon from '@mui/icons-material/Warning';
import Grid from '@mui/material/Grid';
import { upperFirst } from 'lodash-es';

import { VERDICT_STATUS_DISPLAY_MAP } from '@/common/constants/test';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { SpecifiedTestVerdictStatus } from '@/test_verdict/types';

import { useTestVerdict } from '../context';

import { CLInfo } from './cl_info';

function getTestVariantStatusLabel(status: SpecifiedTestVerdictStatus) {
  return upperFirst(VERDICT_STATUS_DISPLAY_MAP[status]);
}

function getTestVariantStatusIcon(status: SpecifiedTestVerdictStatus) {
  switch (status) {
    case TestVariantStatus.UNEXPECTED:
      return <CancelIcon className="unexpected" />;
    case TestVariantStatus.UNEXPECTEDLY_SKIPPED:
      return <ReportIcon className="unexpectedly-skipped" />;
    case TestVariantStatus.FLAKY:
      return <WarningIcon className="flaky" />;
    case TestVariantStatus.EXONERATED:
      return <RemoveCircleIcon className="exonerated" />;
    case TestVariantStatus.EXPECTED:
      return <CheckCircleIcon className="expected" />;
    default:
      return <QuestionMarkIcon className="unspecified" />;
  }
}

export function TestIdentifier() {
  const { status, testId } = useTestVerdict();

  return (
    <Grid item container rowGap={1}>
      <CLInfo />
      <Grid
        item
        container
        columnGap={1}
        alignItems="center"
        sx={{
          fontSize: '1.2rem',
          fontWeight: '700',
          mb: 1,
        }}
      >
        {getTestVariantStatusIcon(status)}
        {getTestVariantStatusLabel(status)}: {testId}
      </Grid>
    </Grid>
  );
}
