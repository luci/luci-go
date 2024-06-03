// Copyright 2022 The LUCI Authors.
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

import { Fragment, useState } from 'react';

import { StringPair } from '@/common/services/common';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';

export interface TagsEntryProps {
  readonly tags: readonly StringPair[];
  readonly ruler?: 'visible' | 'invisible' | 'none';
}

export function TagsEntry({ tags, ruler }: TagsEntryProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader
        onToggle={setExpanded}
        sx={{ overflow: 'hidden', textOverflow: 'ellipsis' }}
      >
        Tags:
        {!expanded && (
          <span css={{ color: 'var(--greyed-out-text-color)' }}>
            {tags.map((tag, i) => (
              <Fragment key={i}>
                <span>{' ' + tag.key}</span>: <span>{tag.value}</span>
                {i !== tags.length - 1 && ','}
              </Fragment>
            ))}
          </span>
        )}
      </ExpandableEntryHeader>
      <ExpandableEntryBody ruler={ruler}>
        <table css={{ width: 'fit-content', overflow: 'hidden' }}>
          <tbody>
            {tags.map((tag, i) => (
              <tr key={i}>
                <td>{tag.key}:</td>
                <td>
                  {tag.value?.match(/^https?:\/\//i) ? (
                    <a href={tag.value} target="_blank" rel="noreferrer">
                      {tag.value}
                    </a>
                  ) : (
                    tag.value
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}
