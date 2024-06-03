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

import { useState } from 'react';

import { OutputTestVerdict } from '@/analysis/types';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { VerdictStatusIcon } from '@/test_verdict/components/verdict_status_icon';
import { SpecifiedTestVerdictStatus } from '@/test_verdict/types';

import { EntryContent } from './entry_content';

export interface TestVerdictEntryProps {
  readonly verdict: OutputTestVerdict;
}

export function TestVerdictEntry({ verdict }: TestVerdictEntryProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader
        onToggle={() => setExpanded(!expanded)}
        sx={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '5px' }}
      >
        <VerdictStatusIcon
          status={SpecifiedTestVerdictStatus.fromAnalysis(verdict.status)}
        />
        {verdict.invocationId}
      </ExpandableEntryHeader>
      <ExpandableEntryBody>
        <EntryContent verdict={verdict} />
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}
