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

import { Skeleton, TableCell } from '@mui/material';

import { VerdictStatusIcon } from '@/common/components/verdict_status_icon';
import { useSetExpanded } from '@/gitiles/components/commit_table';
import { TestVerdict_Status as Analysis_TestVerdict_Status } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import {
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

import { FocusTarget, useSetFocusTarget } from './row_state_provider';

export function SourceVerdictStatusHeadCell() {
  return (
    <TableCell
      width="1px"
      title="Status of source verdict associated with this commit."
    >
      S. V.
    </TableCell>
  );
}

export interface SourceVerdictStatusContentCellProps {
  readonly status: Analysis_TestVerdict_Status;
  /**
   * When `isLoading` is false and `sourceVerdict` is `null`, this entry does
   * not have an associated source verdict.
   */
  readonly isLoading: boolean;
}

export function SourceVerdictStatusContentCell({
  status,
  isLoading,
}: SourceVerdictStatusContentCellProps) {
  const setExpanded = useSetExpanded();
  const setFocusTarget = useSetFocusTarget();

  // Convert from LUCI Analysis status to ResultDB status.
  // At time of writing, the enums are aligned 1:1 so there is
  // no conversion issue.
  const rdbStatus: TestVerdict_Status = status;
  return (
    <TableCell sx={{ minWidth: '30px' }}>
      {rdbStatus !== TestVerdict_Status.STATUS_UNSPECIFIED ? (
        <VerdictStatusIcon
          statusV2={rdbStatus}
          statusOverride={TestVerdict_StatusOverride.NOT_OVERRIDDEN}
          sx={{ verticalAlign: 'middle', cursor: 'pointer' }}
          onClick={() => {
            setExpanded((expanded) => !expanded);
            setFocusTarget(FocusTarget.TestVerdict);
          }}
        />
      ) : isLoading ? (
        <Skeleton variant="circular" height={24} width={24} />
      ) : (
        <></>
      )}
    </TableCell>
  );
}
