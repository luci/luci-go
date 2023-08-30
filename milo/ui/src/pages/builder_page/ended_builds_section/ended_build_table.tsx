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

import { useState } from 'react';

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
import { Build } from '@/common/services/buildbucket';
import { useStore } from '@/common/store';

export interface EndedBuildTableProps {
  readonly endedBuilds: readonly Build[];
  readonly isLoading: boolean;
}

export function EndedBuildTable({
  endedBuilds,
  isLoading,
}: EndedBuildTableProps) {
  // The config is only used during initialization and in handlers.
  // Don't need to declare the component as an observable.
  const config = useStore().userConfig.builderPage;
  const [initDefaultExpanded] = useState(
    () => config.expandEndedBuildsEntryByDefault,
  );

  const hasChanges = endedBuilds.some((b) => b.input?.gerritChanges?.length);

  return (
    <BuildTable
      initDefaultExpanded={initDefaultExpanded}
      onDefaultExpandedChanged={(expand) =>
        config.setExpandEndedBuildsEntryByDefault(expand)
      }
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
    </BuildTable>
  );
}
