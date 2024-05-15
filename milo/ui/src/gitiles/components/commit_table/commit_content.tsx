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
import markdownIt from 'markdown-it';
import { useMemo } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { bugLine } from '@/common/tools/markdown/plugins/bug_line';
import { bugnizerLink } from '@/common/tools/markdown/plugins/bugnizer_link';
import { crbugLink } from '@/common/tools/markdown/plugins/crbug_link';
import { defaultTarget } from '@/common/tools/markdown/plugins/default_target';
import { reviewerLine } from '@/common/tools/markdown/plugins/reviewer_line';

import { useCommit } from './context';

const md = markdownIt('zero', { breaks: true, linkify: true })
  .enable(['linkify', 'newline'])
  .use(bugLine)
  .use(reviewerLine)
  .use(crbugLink)
  .use(bugnizerLink)
  .use(defaultTarget, '_blank');

const SummaryContainer = styled(SanitizedHtml)({
  backgroundColor: 'var(--block-background-color)',
  padding: '4px',
  '& > p:first-of-type': {
    marginBlockStart: 0,
  },
  '& > p:last-of-type': {
    marginBlockEnd: 0,
  },
});

export function CommitContent() {
  const commit = useCommit();

  const { descriptionHtml, changedFiles } = useMemo(
    () => ({
      descriptionHtml: md.render(commit.message),
      changedFiles: commit.treeDiff.map((diff) =>
        // If a file was moved, there is both an old and a new path, from which
        // we take only the new path.
        // If a file was deleted, its new path is /dev/null. In that case, we're
        // only interested in the old path.
        !diff.newPath || diff.newPath === '/dev/null'
          ? diff.oldPath
          : diff.newPath,
      ),
    }),
    [commit],
  );

  return (
    <div css={{ padding: '10px 20px' }}>
      <SummaryContainer html={descriptionHtml} />
      <h4 css={{ marginBlockEnd: '0px' }}>
        Changed files: {changedFiles.length}
      </h4>
      <ul>
        {changedFiles.map((filename, i) => (
          <li key={i}>{filename}</li>
        ))}
      </ul>
    </div>
  );
}
