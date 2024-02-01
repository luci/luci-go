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

import {
  BuildTable,
  BuildTableHead,
  BuildTableRow,
  CreateTimeContentCell,
  CreateTimeHeadCell,
  RunDurationContentCell,
  RunDurationHeadCell,
  StatusContentCell,
  PendingDurationHeadCell,
  StatusHeadCell,
  SummaryContentCell,
  SummaryHeadCell,
  BuildIdentifierContentCell,
  BuildIdentifierHeadCell,
  PendingDurationContentCell,
} from '@/build/components/build_table';
import { BuildTableBody } from '@/build/components/build_table/build_table_body';
import { OutputBuild } from '@/build/types';

export interface RelatedBuildTableProps {
  readonly relatedBuilds: readonly OutputBuild[];
  readonly showLoadingBar?: boolean;
}

export function RelatedBuildTable({
  relatedBuilds,
  showLoadingBar,
}: RelatedBuildTableProps) {
  return (
    <BuildTable initDefaultExpanded>
      <BuildTableHead showLoadingBar={showLoadingBar}>
        <StatusHeadCell />
        <BuildIdentifierHeadCell />
        <CreateTimeHeadCell />
        <PendingDurationHeadCell />
        <RunDurationHeadCell />
        <SummaryHeadCell />
      </BuildTableHead>
      <BuildTableBody>
        {relatedBuilds.map((b) => (
          <BuildTableRow key={b.id} build={b}>
            <StatusContentCell />
            <BuildIdentifierContentCell />
            <CreateTimeContentCell />
            <PendingDurationContentCell />
            <RunDurationContentCell />
            <SummaryContentCell />
          </BuildTableRow>
        ))}
      </BuildTableBody>
    </BuildTable>
  );
}
