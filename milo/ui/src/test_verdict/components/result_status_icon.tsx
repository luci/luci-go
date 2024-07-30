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

import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

const RESULT_STATUS_ICON_FONT_MAP = Object.freeze({
  [TestStatus.ABORT]: 'do_not_disturb_on_total_silence',
  [TestStatus.CRASH]: 'broken_image',
  [TestStatus.FAIL]: 'cancel',
  [TestStatus.PASS]: 'check_circle',
  [TestStatus.SKIP]: 'do_not_disturb',
  [TestStatus.STATUS_UNSPECIFIED]: 'question_mark',
});

const RESULT_STATUS_COLOR_MAP = Object.freeze({
  [TestStatus.ABORT]: 'var(--failure-color)',
  [TestStatus.CRASH]: 'var(--failure-color)',
  [TestStatus.FAIL]: 'var(--failure-color)',
  [TestStatus.PASS]: 'var(--success-color)',
  [TestStatus.SKIP]: 'var(--warning-color)',
  [TestStatus.STATUS_UNSPECIFIED]: 'var(--warning-color)',
});

export interface ResultStatusIconProps {
  readonly status: TestStatus;
  readonly sx?: SxProps<Theme>;
}

export function ResultStatusIcon({ status, sx }: ResultStatusIconProps) {
  return (
    <Icon sx={{ color: RESULT_STATUS_COLOR_MAP[status], ...sx }}>
      {RESULT_STATUS_ICON_FONT_MAP[status]}
    </Icon>
  );
}
