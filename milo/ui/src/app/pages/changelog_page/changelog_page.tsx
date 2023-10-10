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

import { Typography, styled } from '@mui/material';
import { useEffect, useMemo } from 'react';

import { renderChangelog, useChangelog } from '@/app/components/changelog';
import { useMarkChangelogAsRead } from '@/app/components/changelog';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { SanitizedHtml } from '@/common/components/sanitized_html';
import { UiPage } from '@/common/constants';

const ChangelogContainer = styled(SanitizedHtml)({
  '& *': {
    fontSize: '20px',
  },
});

export function ChangelogPage() {
  const changelog = useChangelog();
  const markChangelogAsRead = useMarkChangelogAsRead();
  useEffect(() => markChangelogAsRead(), [markChangelogAsRead]);

  const [latestHtml, pastHtml] = useMemo(
    () => [renderChangelog(changelog.latest), renderChangelog(changelog.past)],
    [changelog],
  );

  return (
    <>
      <PageMeta title={UiPage.WHATS_NEW} />
      <div
        css={{
          padding: '30px',
          marginLeft: 'auto',
          marginRight: 'auto',
          maxWidth: '1000px',
        }}
      >
        <Typography variant="h4">{"What's new?"}</Typography>
        <ChangelogContainer html={latestHtml} />
        <Typography variant="h4" sx={{ mt: 15 }}>
          {'Past changes'}
        </Typography>
        <ChangelogContainer html={pastHtml} />
      </div>
    </>
  );
}

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="changelog">
    <ChangelogPage />
  </RecoverableErrorBoundary>
);
