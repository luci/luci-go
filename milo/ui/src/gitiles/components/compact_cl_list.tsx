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

import { styled } from '@mui/material';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { ChangelistLink } from '@/gitiles/components/changelist_link';
import { GerritChange } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

const ListContainer = styled('ul')`
  padding: 10px 10px 10px 20px;
  margin: 0px;
`;

export interface CompactClListProps {
  readonly changes: readonly GerritChange[];
}

export function CompactClList({ changes }: CompactClListProps) {
  if (changes.length === 0) {
    return;
  }

  return (
    // Wrap the whole thing in a tooltip so it's more natural to display the
    // entire list of changes rather than just the elided ones.
    //
    // We want to display the entire list because in the future we might want to
    // add more details to the tooltip (e.g. CL status) and those details should
    // be displayed for the first CL as well.
    <HtmlTooltip
      title={
        <ListContainer>
          {changes.map((cl, i) => (
            <li key={i}>
              <ChangelistLink changelist={cl} />
            </li>
          ))}
        </ListContainer>
      }
    >
      <span>
        <ChangelistLink changelist={changes[0]} />
        {changes.length > 1 && <span> + {changes.length - 1} more</span>}
      </span>
    </HtmlTooltip>
  );
}
