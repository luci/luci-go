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

import { observer } from 'mobx-react-lite';
import { useMemo } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { BUILD_STATUS_DISPLAY_MAP } from '@/common/constants/legacy';
import { useStore } from '@/common/store';
import { renderMarkdown } from '@/common/tools/markdown/utils';

export const SummarySection = observer(() => {
  const store = useStore();
  const build = store.buildPage.build;

  const summaryHtml = useMemo(
    () =>
      build?.data.summaryMarkdown
        ? renderMarkdown(build?.data.summaryMarkdown)
        : null,
    [build?.data.summaryMarkdown],
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
            Build{' '}
            {BUILD_STATUS_DISPLAY_MAP[build.data.status] || 'status unknown'}
          </div>
        )}
      </div>
    </>
  );
});
