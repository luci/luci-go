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

import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { FailureReason } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';

export interface FailureReasonEntryProps {
  readonly failureReason: FailureReason;
}

export function FailureReasonEntry({ failureReason }: FailureReasonEntryProps) {
  const [expanded, setExpanded] = useState(true);

  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader onToggle={(expanded) => setExpanded(expanded)}>
        Failure Reason:
      </ExpandableEntryHeader>
      <ExpandableEntryBody>
        <pre
          css={{
            backgroundColor: 'var(--block-background-color)',
            padding: '5px',
            margin: 0,
          }}
        >
          {failureReason.primaryErrorMessage}
        </pre>
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}
