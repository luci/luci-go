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

import createCache, { EmotionCache } from '@emotion/cache';
import { CacheProvider } from '@emotion/react';
import { styled } from '@mui/material';

import { getUniqueBugs } from '@/analysis/tools/cluster_utils';
import { OutputClusterEntry } from '@/analysis/types';
import { BugCard } from '@/common/components/bug_card';
import { HtmlTooltip } from '@/common/components/html_tooltip';

const Badge = styled('span')`
  margin: 0;
  background-color: #b7b7b7;
  width: 100%;
  box-sizing: border-box;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: middle;
  color: white;
  padding: 0.25em 0.4em;
  font-size: 75%;
  font-weight: 700;
  line-height: 16px;
  text-align: center;
  white-space: nowrap;
  border-radius: 0.25rem;
`;

export interface AssociatedBugsBadgeProps {
  readonly project: string;
  readonly clusters: readonly OutputClusterEntry[];
  readonly cache?: EmotionCache;
}

export function AssociatedBugsBadge({
  project,
  clusters,
  cache = createCache({
    key: 'milo-associated-bugs-badge-react',
    container: document.body,
  }),
}: AssociatedBugsBadgeProps) {
  const uniqueBugs = getUniqueBugs(
    clusters.flatMap((c) => (c.bug ? [c.bug] : [])),
  );
  if (!uniqueBugs.length) {
    return <></>;
  }

  return (
    <>
      {uniqueBugs.map((b) => (
        <HtmlTooltip
          key={b.id}
          title={
            <BugCard
              bugId={b.id}
              project={project}
              clusterId={clusters.find((c) => c.bug?.id === b.id)?.clusterId}
            />
          }
        >
          <span>
            <CacheProvider value={cache}>
              <Badge data-testid="associated-bugs-badge">{b.linkText}</Badge>
            </CacheProvider>
          </span>
        </HtmlTooltip>
      ))}
    </>
  );
}
