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

import { styled } from '@mui/material';

import {
  BuildTable,
  BuildTableHead,
  BuildTableRow,
  CreateTimeContentCell,
  CreateTimeHeadCell,
  RunDurationContentCell,
  RunDurationHeadCell,
  PendingDurationHeadCell,
  SummaryContentCell,
  SummaryHeadCell,
  PendingDurationContentCell,
} from '@/build/components/build_table';
import {
  BuildIdentifierContentCell,
  BuildIdentifierHeadCell,
} from '@/build/components/build_table/build_identifier_column';
import { BuildTableBody } from '@/build/components/build_table/build_table_body';
import { OutputBuild } from '@/build/types';
import { ReadonlyCategoryTree } from '@/generic_libs/tools/category_tree';

const StyledBuildTableRow = styled(BuildTableRow)`
  &.selected {
    position: relative;
  }
  &.selected:after {
    z-index: -1;
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 34px;
    width: 100%;
    background-color: rgba(25, 118, 210);
    mask-image: linear-gradient(
      90deg,
      rgb(0 0 0 / 15%),
      rgb(0 0 0 / 15%) 300px,
      rgb(0 0 0 / 0%) 400px,
      rgb(0 0 0 / 0%)
    );
  }
`;

export interface RelatedBuildTableProps {
  readonly selfBuild: OutputBuild;
  readonly buildTree: ReadonlyCategoryTree<string, OutputBuild>;
  readonly showLoadingBar?: boolean;
}

export function RelatedBuildTable({
  selfBuild,
  buildTree,
  showLoadingBar,
}: RelatedBuildTableProps) {
  return (
    <BuildTable initDefaultExpanded>
      <BuildTableHead showLoadingBar={showLoadingBar}>
        <BuildIdentifierHeadCell />
        <CreateTimeHeadCell />
        <PendingDurationHeadCell />
        <RunDurationHeadCell />
        <SummaryHeadCell />
      </BuildTableHead>
      <BuildTableBody>
        {[...buildTree.enumerate()].map(([index, b]) => (
          <StyledBuildTableRow
            key={b.id}
            build={b}
            className={selfBuild.id === b.id ? 'selected' : ''}
          >
            <BuildIdentifierContentCell
              defaultProject={selfBuild.builder.project}
              indentLevel={index.length - 1}
              selected={selfBuild.id === b.id}
            />
            <CreateTimeContentCell />
            <PendingDurationContentCell />
            <RunDurationContentCell />
            <SummaryContentCell />
          </StyledBuildTableRow>
        ))}
      </BuildTableBody>
    </BuildTable>
  );
}
