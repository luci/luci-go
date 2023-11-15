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

import { TestVariantStatus } from '@/common/services/resultdb';

import { useTestVerdict } from '../context';

import { CLInfo } from './cl_info';

function getTestStatusIconLabel(status: TestVariantStatus) {
  switch (status) {
    case TestVariantStatus.UNEXPECTED:
      return (
        <>
          <CancelIcon className="unexpected" /> Unexpectedly failed:
        </>
      );
    case TestVariantStatus.UNEXPECTEDLY_SKIPPED:
      return (
        <>
          <ReportIcon className="unexpectedly-skipped" /> Unexpectedly skipped:
        </>
      );
    case TestVariantStatus.FLAKY:
      return (
        <>
          <WarningIcon className="flaky" /> Flaky:
        </>
      );
    case TestVariantStatus.EXONERATED:
      return (
        <>
          <RemoveCircleIcon className="exonerated" /> Exonerated:
        </>
      );
    case TestVariantStatus.EXPECTED:
      return (
        <>
          <CheckCircleIcon className="expected" /> Expected:
        </>
      );
    case TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED:
    default:
      return (
        <>
          <QuestionMarkIcon className="unspecified" /> Unknown status:
        </>
      );
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
        }}
      >
        {getTestStatusIconLabel(status)} {testId}
      </Grid>
    </Grid>
  );
}
