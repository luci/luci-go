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

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { SanitizedHtml } from '@/common/components/sanitized_html';
import { useMarkReleaseNotesRead } from '@/core/components/release_notes';
import {
  renderReleaseNotes,
  useReleaseNotes,
} from '@/core/components/release_notes';

const ReleaseNotesContainer = styled(SanitizedHtml)({
  '& *': {
    fontSize: '20px',
  },
  '& h1': {
    fontSize: '30px',
    fontWeight: 400,
  },
});

export function ReleaseNotesPage() {
  const releaseNotes = useReleaseNotes();
  const markReleaseNotesAsRead = useMarkReleaseNotesRead();
  useEffect(() => markReleaseNotesAsRead(), [markReleaseNotesAsRead]);

  const [latestHtml, pastHtml] = useMemo(
    () => [
      renderReleaseNotes(releaseNotes.latest),
      renderReleaseNotes(releaseNotes.past),
    ],
    [releaseNotes],
  );

  return (
    <>
      <PageMeta title="What's new" />
      <div
        css={{
          padding: '30px',
          marginLeft: 'auto',
          marginRight: 'auto',
          maxWidth: '1000px',
        }}
      >
        <Typography variant="h4">{"What's new?"}</Typography>
        <ReleaseNotesContainer html={latestHtml} />
        {/* Temporarily move the "Past releases" heading to RELEASE_NOTES.md
         ** itself so we can create dummy release tag to divide summary and
         ** details.
         **
         ** TODO: move the "Past releases" heading back once we have a formal
         ** mechanism for annotating highlights.
         **/}
        {/* <Typography variant="h4" sx={{ mt: 15 }}>
          {'Past releases'}
        </Typography> */}
        <ReleaseNotesContainer html={pastHtml} />
      </div>
    </>
  );
}

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="release-notes">
    <ReleaseNotesPage />
  </RecoverableErrorBoundary>
);
