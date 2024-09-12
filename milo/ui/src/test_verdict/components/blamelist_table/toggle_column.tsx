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

import {
  ToggleContentCell,
  ToggleContentCellProps,
} from '@/gitiles/components/commit_table';

import { FocusTarget, useSetFocusTarget } from './row_state_provider';

/**
 * Similar to <ToggleContentCell /> except that it also signals that the commit
 * entry should be expanded.
 */
export function CommitToggleContentCell({
  onToggle,
  ...props
}: ToggleContentCellProps) {
  const setFocusTarget = useSetFocusTarget();

  return (
    <ToggleContentCell
      {...props}
      onToggle={(expand) => {
        setFocusTarget(FocusTarget.Commit);
        onToggle?.(expand);
      }}
    />
  );
}
