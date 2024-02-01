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

import { cleanup, render, screen } from '@testing-library/react';

import {
  ReleaseNotesProvider,
  useHasNewRelease,
} from '@/core/components/release_notes';
import { PageMetaProvider } from '@/common/components/page_meta/page_meta_provider';

import { ReleaseNotesPage } from './release_notes_page';

function ShowUnreadReleaseNotesComponent() {
  const hasNew = useHasNewRelease();
  return <>{hasNew ? 'has new release notes' : 'no new release notes'}</>;
}

describe('ReleaseNotesPage', () => {
  afterEach(() => {
    cleanup();
    localStorage.clear();
  });

  it('marks release notes as read', () => {
    render(
      <PageMetaProvider>
        <ReleaseNotesProvider
          initReleaseNotes={{ latest: '', latestVersion: 10, past: '' }}
        >
          <ShowUnreadReleaseNotesComponent />
          <ReleaseNotesPage />
        </ReleaseNotesProvider>
      </PageMetaProvider>,
    );
    expect(screen.queryByText('has new release notes')).not.toBeInTheDocument();
  });
});
