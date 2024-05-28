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

import { useLocalStorage } from 'react-use';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';

const EXPANDED_CACHE_KEY = 'builder-description-section-expanded';

export interface BuilderDescriptionSectionProps {
  readonly descriptionHtml: string;
}

export function BuilderDescriptionSection({
  descriptionHtml,
}: BuilderDescriptionSectionProps) {
  const [expanded = true, setExpanded] =
    useLocalStorage<boolean>(EXPANDED_CACHE_KEY);

  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader sx={{ lineHeight: '19px' }} onToggle={setExpanded}>
        <h3>Builder Description</h3>
      </ExpandableEntryHeader>
      <ExpandableEntryBody ruler="none">
        <SanitizedHtml
          html={descriptionHtml}
          sx={{
            padding: '10px',
            backgroundColor: 'var(--block-background-color)',
          }}
        />
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}
