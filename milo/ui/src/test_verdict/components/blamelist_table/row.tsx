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

import { ParsedTestVariantBranchName } from '@/analysis/types';
import {
  AuthorContentCell,
  AuthorHeadCell,
  CommitTableRow,
  PositionContentCell,
  PositionHeadCell,
  TimeContentCell,
  TimeHeadCell,
  TitleContentCell,
  TitleHeadCell,
  ToggleHeadCell,
} from '@/gitiles/components/commit_table';
import { OutputCommit } from '@/gitiles/types';
import { QuerySourceVerdictsResponse_SourceVerdict } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { EntryContent } from './entry_content';
import { BlamelistRowStateProvider } from './row_state_provider';
import { SegmentContentCell, SegmentHeadCell } from './segment_column';
import {
  SourceVerdictStatusContentCell,
  SourceVerdictStatusHeadCell,
} from './source_verdict_status_column';
import { CommitToggleContentCell } from './toggle_column';

export function BlamelistTableHeaderContent() {
  return (
    <>
      <SegmentHeadCell />
      <ToggleHeadCell hotkey="x" />
      <SourceVerdictStatusHeadCell />
      <PositionHeadCell />
      <TimeHeadCell />
      <AuthorHeadCell />
      <TitleHeadCell />
    </>
  );
}

export interface BlamelistTableRowProps {
  readonly testVariantBranch: ParsedTestVariantBranchName;
  readonly commit: OutputCommit | null;
  readonly position: string;
  readonly sourceVerdict: QuerySourceVerdictsResponse_SourceVerdict | null;
  readonly isSvLoading: boolean;
}

export function BlamelistTableRow({
  testVariantBranch,
  commit,
  position,
  sourceVerdict,
  isSvLoading,
}: BlamelistTableRowProps) {
  return (
    <BlamelistRowStateProvider>
      <CommitTableRow
        commit={commit}
        content={
          <EntryContent
            testId={testVariantBranch.testId}
            variantHash={testVariantBranch.variantHash}
            sourceVerdict={sourceVerdict}
            isSvLoading={isSvLoading}
          />
        }
      >
        <SegmentContentCell position={position} />
        <CommitToggleContentCell />
        <SourceVerdictStatusContentCell
          status={sourceVerdict?.status || null}
          isLoading={isSvLoading}
        />
        <PositionContentCell position={position} />
        <TimeContentCell />
        <AuthorContentCell />
        <TitleContentCell />
      </CommitTableRow>
    </BlamelistRowStateProvider>
  );
}
