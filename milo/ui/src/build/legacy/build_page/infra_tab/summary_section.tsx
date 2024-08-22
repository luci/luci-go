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

import { useMemo } from 'react';

import { BUILD_STATUS_DISPLAY_MAP } from '@/build/constants';
import { SanitizedHtml } from '@/common/components/sanitized_html';
import { renderMarkdown } from '@/common/tools/markdown/utils';

import { useBuild } from '../hooks';

export function SummarySection() {
  const build = useBuild();

  const summaryHtml = useMemo(
    () =>
      build?.summaryMarkdown ? renderMarkdown(build.summaryMarkdown) : null,
    [build?.summaryMarkdown],
  );

  if (!build) {
    return <></>;
  }

  return (
    <>
      <div
        css={{
          padding: '5px 10px',
          clear: 'both',
          overflowWrap: 'break-word',
          '& pre': {
            whiteSpace: 'pre-wrap',
            overflowWrap: 'break-word',
            fontSize: '12px',
          },
          '& *': {
            marginBlock: '10px',
          },
          backgroundColor: 'var(--block-background-color)',
        }}
      >
        {summaryHtml ? (
          <SanitizedHtml html={summaryHtml} />
        ) : (
          <div css={{ fontWeight: 500 }}>
            Build {BUILD_STATUS_DISPLAY_MAP[build.status]}
          </div>
        )}
      </div>
    </>
  );
}
