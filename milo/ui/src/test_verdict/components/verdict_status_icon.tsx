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

import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { VERDICT_STATUS_COLOR_MAP } from '@/test_verdict/constants/verdict';
import { SpecifiedTestVerdictStatus } from '@/test_verdict/types';

const VERDICT_STATUS_ICON_FONT_MAP = Object.freeze({
  [TestVariantStatus.EXONERATED]: 'remove_circle',
  [TestVariantStatus.EXPECTED]: 'check_circle',
  [TestVariantStatus.FLAKY]: 'warning',
  [TestVariantStatus.UNEXPECTED]: 'cancel',
  [TestVariantStatus.UNEXPECTEDLY_SKIPPED]: 'report',
});

export interface VerdictStatusIconProps {
  readonly status: SpecifiedTestVerdictStatus;
  readonly sx?: SxProps<Theme>;
}

export function VerdictStatusIcon({ status, sx }: VerdictStatusIconProps) {
  return (
    <Icon sx={{ color: VERDICT_STATUS_COLOR_MAP[status], ...sx }}>
      {VERDICT_STATUS_ICON_FONT_MAP[status]}
    </Icon>
  );
}
