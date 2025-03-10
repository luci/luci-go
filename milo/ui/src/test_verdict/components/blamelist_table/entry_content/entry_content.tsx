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

import { Skeleton } from '@mui/material';
import { useState } from 'react';

import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { CommitContent } from '@/gitiles/components/commit_table';
import { QuerySourceVerdictsResponse_SourceVerdict } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { TestVerdictEntry } from '@/test_verdict/components/test_verdict_entry';

import { useProject } from '../context';
import { FocusTarget, useFocusTarget } from '../row_state_provider';

export interface EntryContentProps {
  readonly testId: string;
  readonly variantHash: string;
  readonly sourceVerdict: QuerySourceVerdictsResponse_SourceVerdict | null;
  /**
   * When `isSvLoading` is false and `sourceVerdict` is `null`, this entry does
   * not have an associated source verdict.
   */
  readonly isSvLoading: boolean;
}

export function EntryContent({
  testId,
  variantHash,
  sourceVerdict,
  isSvLoading,
}: EntryContentProps) {
  const project = useProject();
  const [expanded, setExpanded] = useState(true);
  const focusTarget = useFocusTarget();

  return (
    <>
      {sourceVerdict ? (
        sourceVerdict.verdicts.map((v, i) => (
          <TestVerdictEntry
            key={v.invocationId}
            project={project}
            testId={testId}
            variantHash={variantHash}
            invocationId={v.invocationId}
            changes={v.changelists.map((c) => ({
              host: c.host,
              change: c.change,
              patchset: c.patchset.toString(),
            }))}
            // If the user clicks on the verdict status icon to expand the
            // blamelist table row, they likely want to see the verdicts. Expand
            // the first verdict entry.
            defaultExpanded={i === 0 && focusTarget === FocusTarget.TestVerdict}
          />
        ))
      ) : isSvLoading ? (
        <Skeleton />
      ) : (
        <></>
      )}
      <ExpandableEntry expanded={expanded}>
        <ExpandableEntryHeader onToggle={(expand) => setExpanded(expand)}>
          Commit
        </ExpandableEntryHeader>
        <ExpandableEntryBody>
          <CommitContent sx={{ paddingLeft: 0 }} />
        </ExpandableEntryBody>
      </ExpandableEntry>
    </>
  );
}
