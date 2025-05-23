// Copyright 2024 The LUCI Authors.
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

import { Icon, SxProps, Theme } from '@mui/material';

import {
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import {
  VERDICT_STATUS_COLOR_MAP,
  VERDICT_STATUS_OVERRIDE_COLOR_MAP,
} from '@/test_verdict/constants/verdict';
import {
  SpecifiedTestVerdictStatus,
  SpecifiedTestVerdictStatusOverride,
} from '@/test_verdict/types';

const VERDICT_STATUS_ICON_FONT_MAP = Object.freeze({
  [TestVerdict_Status.FAILED]: 'cancel',
  [TestVerdict_Status.EXECUTION_ERRORED]: 'report',
  [TestVerdict_Status.PRECLUDED]: 'not_started',
  [TestVerdict_Status.FLAKY]: 'warning',
  [TestVerdict_Status.PASSED]: 'check_circle',
  [TestVerdict_Status.SKIPPED]: 'next_plan',
});

const VERDICT_STATUS_OVERRIDE_ICON_FONT_MAP = Object.freeze({
  [TestVerdict_StatusOverride.EXONERATED]: 'remove_circle',
});

export interface VerdictStatusIconProps {
  readonly statusV2: SpecifiedTestVerdictStatus;
  readonly statusOverride?: SpecifiedTestVerdictStatusOverride;
  readonly sx?: SxProps<Theme>;
}

export function VerdictStatusIcon({
  statusV2,
  statusOverride,
  sx,
}: VerdictStatusIconProps) {
  if (
    statusOverride !== undefined &&
    statusOverride !== TestVerdict_StatusOverride.NOT_OVERRIDDEN
  ) {
    return (
      <Icon
        sx={{ color: VERDICT_STATUS_OVERRIDE_COLOR_MAP[statusOverride], ...sx }}
      >
        {VERDICT_STATUS_OVERRIDE_ICON_FONT_MAP[statusOverride]}
      </Icon>
    );
  }
  return (
    <Icon sx={{ color: VERDICT_STATUS_COLOR_MAP[statusV2], ...sx }}>
      {VERDICT_STATUS_ICON_FONT_MAP[statusV2]}
    </Icon>
  );
}
