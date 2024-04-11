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

import { Box, Link } from '@mui/material';
import markdownIt from 'markdown-it';
import { useMemo, useState } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { bugLine } from '@/common/tools/markdown/plugins/bug_line';
import { bugnizerLink } from '@/common/tools/markdown/plugins/bugnizer_link';
import { crbugLink } from '@/common/tools/markdown/plugins/crbug_link';
import { defaultTarget } from '@/common/tools/markdown/plugins/default_target';
import { reviewerLine } from '@/common/tools/markdown/plugins/reviewer_line';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { SourceRef } from '@/proto/go.chromium.org/luci/analysis/proto/v1/sources.pb';
import { SourcePosition } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { VerdictsStatusIcon } from './verdicts_status_icon';

const md = markdownIt('zero', { breaks: true, linkify: true })
  .enable(['linkify', 'newline'])
  .use(bugLine)
  .use(reviewerLine)
  .use(crbugLink)
  .use(bugnizerLink)
  .use(defaultTarget, '_blank');

export interface CommitRowEntryProps {
  readonly sourcePosition: SourcePosition;
  readonly sourceRef?: SourceRef;
}

export function CommitRowEntry({
  sourcePosition,
  sourceRef,
}: CommitRowEntryProps) {
  const [expanded, setExpanded] = useState(false);
  const { descriptionHtml, changedFiles } = useMemo(
    () => ({
      descriptionHtml: md.render(sourcePosition?.commit?.message || ''),
      changedFiles: sourcePosition?.commit?.treeDiff.map((diff) =>
        // If a file was moved, there is both an old and a new path, from which
        // we take only the new path.
        // If a file was deleted, its new path is /dev/null. In that case, we're
        // only interested in the old path.
        !diff.newPath || diff.newPath === '/dev/null'
          ? diff.oldPath
          : diff.newPath,
      ),
    }),
    [sourcePosition.commit],
  );
  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader
        sx={{
          display: 'grid',
          gridTemplateColumns: 'var(--commit-columns)',
          gridGap: '5px',
          lineHeight: '24px',
        }}
        onToggle={setExpanded}
      >
        <Box>
          <VerdictsStatusIcon testVerdicts={[...sourcePosition.verdicts]} />
        </Box>

        <Box>
          <Link
            href={`https://${sourceRef?.gitiles?.host.split(
              '.',
            )[0]}-review.googlesource.com/q/${sourcePosition.commit?.id}`}
            target="_blank"
          >
            {sourcePosition.position}
          </Link>
        </Box>
        <Box>{sourcePosition.commit?.author?.time}</Box>
        <Box sx={{ textOverflow: 'ellipsis', overflow: 'hidden' }}>
          {sourcePosition.commit?.author?.email}
        </Box>
        <Box>{sourcePosition.commit?.message.split('\n')[0]}</Box>
      </ExpandableEntryHeader>
      <ExpandableEntryBody>
        <div>
          <CommitEntry
            message={descriptionHtml}
            changedFiles={changedFiles || []}
          />
          {/* TODO: Display verdicts and test results*/}
        </div>
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}

export interface CommitEntryProps {
  readonly message: string;
  readonly changedFiles: string[];
}

function CommitEntry({ message, changedFiles }: CommitEntryProps) {
  const [expanded, setExpanded] = useState(true);
  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader onToggle={setExpanded}>
        Commit
      </ExpandableEntryHeader>
      <ExpandableEntryBody>
        <div>
          <SanitizedHtml
            sx={{
              backgroundColor: 'var(--block-background-color)',
              padding: '5px',
            }}
            html={message}
          />
          <h4 css={{ marginBlockEnd: '0px' }}>
            Changed files: {changedFiles.length}
          </h4>
          <ul>
            {changedFiles.map((filename, i) => (
              <li key={i}>{filename}</li>
            ))}
          </ul>
        </div>
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}
