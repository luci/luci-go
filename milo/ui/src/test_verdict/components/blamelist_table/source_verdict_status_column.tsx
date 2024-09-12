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

import { SourceVerdictStatusIcon } from '@/analysis/components/source_verdict_status_icon';
import { SpecifiedSourceVerdictStatus } from '@/analysis/types';
import { useSetExpanded } from '@/gitiles/components/commit_table';

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
  readonly status: SpecifiedSourceVerdictStatus | null;
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

  return (
    <TableCell sx={{ minWidth: '30px' }}>
      {status ? (
        <SourceVerdictStatusIcon
          status={status}
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
