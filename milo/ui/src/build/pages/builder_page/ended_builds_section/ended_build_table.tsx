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

import { memo } from 'react';
import { useLocalStorage } from 'react-use';

import {
  BuildNumContentCell,
  BuildNumHeadCell,
  BuildTable,
  BuildTableHead,
  BuildTableRow,
  CommitContentCell,
  CommitHeadCell,
  CreateTimeContentCell,
  CreateTimeHeadCell,
  EndTimeContentCell,
  EndTimeHeadCell,
  GerritChangesContentCell,
  GerritChangesHeadCell,
  RunDurationContentCell,
  RunDurationHeadCell,
  StatusContentCell,
  StatusHeadCell,
  SummaryContentCell,
  SummaryHeadCell,
} from '@/build/components/build_table';
import { BuildTableBody } from '@/build/components/build_table/build_table_body';
import { OutputBuild } from '@/build/types';

const ENDED_BUILDS_TABLE_DEFAULT_EXPANDED =
  'ended-builds-table-default-expanded';

interface EndedBuildTableBodyProps {
  readonly endedBuilds: readonly OutputBuild[];
  readonly hasChanges: boolean;
}

// The table body can be large and expensive to render.
// Use `memo` to stop it from rerendering when only the default expansion state
// and/or the loading status were changed.
const EndedBuildTableBody = memo(function EndedBuildTableBody({
  endedBuilds,
  hasChanges,
}: EndedBuildTableBodyProps) {
  return (
    <BuildTableBody>
      {endedBuilds.map((b) => (
        <BuildTableRow key={b.id} build={b}>
          <StatusContentCell />
          <BuildNumContentCell />
          <CreateTimeContentCell />
          <EndTimeContentCell />
          <RunDurationContentCell />
          <CommitContentCell />
          {hasChanges && <GerritChangesContentCell />}
          <SummaryContentCell />
        </BuildTableRow>
      ))}
    </BuildTableBody>
  );
});

export interface EndedBuildTableProps {
  readonly endedBuilds: readonly OutputBuild[];
  readonly isLoading: boolean;
}

export function EndedBuildTable({
  endedBuilds,
  isLoading,
}: EndedBuildTableProps) {
  const [defaultExpanded = true, setDefaultExpanded] = useLocalStorage<boolean>(
    ENDED_BUILDS_TABLE_DEFAULT_EXPANDED,
  );

  const hasChanges = endedBuilds.some((b) => b.input?.gerritChanges?.length);

  return (
    <BuildTable
      initDefaultExpanded={defaultExpanded}
      onDefaultExpandedChanged={(expand) => setDefaultExpanded(expand)}
    >
      <BuildTableHead showLoadingBar={isLoading}>
        <StatusHeadCell />
        <BuildNumHeadCell />
        <CreateTimeHeadCell />
        <EndTimeHeadCell />
        <RunDurationHeadCell />
        <CommitHeadCell />
        {hasChanges && <GerritChangesHeadCell />}
        <SummaryHeadCell />
      </BuildTableHead>
      <EndedBuildTableBody endedBuilds={endedBuilds} hasChanges={hasChanges} />
    </BuildTable>
  );
}
