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

import { Icon, IconProps } from '@mui/material';

import { SpecifiedSourceVerdictStatus } from '@/analysis/types';
import { QuerySourceVerdictsResponse_VerdictStatus } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

const VERDICT_STATUS_ICON_FONT_MAP = Object.freeze({
  [QuerySourceVerdictsResponse_VerdictStatus.EXPECTED]: 'check_circle',
  [QuerySourceVerdictsResponse_VerdictStatus.FLAKY]: 'warning',
  [QuerySourceVerdictsResponse_VerdictStatus.UNEXPECTED]: 'cancel',
  [QuerySourceVerdictsResponse_VerdictStatus.SKIPPED]: 'do_not_disturb',
});

const VERDICT_STATUS_COLOR_MAP = Object.freeze({
  [QuerySourceVerdictsResponse_VerdictStatus.EXPECTED]: 'var(--success-color)',
  [QuerySourceVerdictsResponse_VerdictStatus.FLAKY]: 'var(--warning-color)',
  [QuerySourceVerdictsResponse_VerdictStatus.UNEXPECTED]:
    'var(--failure-color)',
  [QuerySourceVerdictsResponse_VerdictStatus.SKIPPED]:
    'var(--exonerated-color)',
});

export interface SourceVerdictStatusIconProps extends IconProps {
  readonly status: SpecifiedSourceVerdictStatus;
}

export function SourceVerdictStatusIcon({
  status,
  sx,
  ...props
}: SourceVerdictStatusIconProps) {
  return (
    <Icon {...props} sx={{ color: VERDICT_STATUS_COLOR_MAP[status], ...sx }}>
      {VERDICT_STATUS_ICON_FONT_MAP[status]}
    </Icon>
  );
}
