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

import { TableCell } from '@mui/material';

import { TestVerdict } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { VerdictsStatusIcon } from '@/test_verdict/components/verdicts_status_icon';

export function VerdictsStatusHeadCell() {
  return (
    <TableCell
      width="1px"
      title="Status of test verdicts associated with this commit."
    >
      Verd.
    </TableCell>
  );
}

export interface VerdictsStatusContentCellProps {
  readonly testVerdicts: readonly TestVerdict[];
}

export function VerdictStatusesContentCell({
  testVerdicts,
}: VerdictsStatusContentCellProps) {
  return (
    <TableCell>
      <VerdictsStatusIcon testVerdicts={testVerdicts} />
    </TableCell>
  );
}