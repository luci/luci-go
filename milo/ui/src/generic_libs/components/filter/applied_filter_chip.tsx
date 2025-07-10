// Copyright 2025 The LUCI Authors.
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

import { BaseFilterChip } from './base_filter_chip';

interface AppliedFilterChipProps {
  filterKey: string;
  filterValue: string;
  onRemove: () => void;
}

/**
 * A simple chip for displaying an active key-value filter and allowing its removal.
 */
export function AppliedFilterChip({
  filterKey,
  filterValue,
  onRemove,
}: AppliedFilterChipProps) {
  return (
    <BaseFilterChip
      label={`${filterKey}: ${filterValue}`}
      active={true}
      onClear={onRemove}
    />
  );
}
