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
import { useEffect } from 'react';

import { getLastReadVersion } from './common';
import { ReleaseNotesProvider } from './context';
import { useHasNewRelease, useMarkReleaseNotesRead } from './hooks';

function ReadReleaseNotesComponent() {
  const markAsRead = useMarkReleaseNotesRead();
  useEffect(() => markAsRead(), [markAsRead]);
  return <></>;
}

function ShowUnreadReleaseNotesComponent() {
  const hasNew = useHasNewRelease();
  return <>{hasNew ? 'has new release notes' : 'no new release notes'}</>;
}

describe('ReleaseNotesProvider', () => {
  afterEach(() => {
    cleanup();
    localStorage.clear();
  });

  it('can mark release notes as read', () => {
    const { rerender } = render(
      <ReleaseNotesProvider
        initReleaseNotes={{ latest: '', latestVersion: 10, past: '' }}
      >
        <ShowUnreadReleaseNotesComponent />
      </ReleaseNotesProvider>,
    );
    expect(getLastReadVersion()).toBe(-1);
    expect(screen.getByText('has new release notes')).toBeInTheDocument();

    rerender(
      <ReleaseNotesProvider
        initReleaseNotes={{ latest: '', latestVersion: 10, past: '' }}
      >
        <ReadReleaseNotesComponent />
      </ReleaseNotesProvider>,
    );
    expect(getLastReadVersion()).toBe(10);

    rerender(
      <ReleaseNotesProvider
        initReleaseNotes={{ latest: '', latestVersion: 10, past: '' }}
      >
        <ShowUnreadReleaseNotesComponent />
      </ReleaseNotesProvider>,
    );

    expect(screen.getByText('no new release notes')).toBeInTheDocument();
  });

  it('can handle rollback', () => {
    let { rerender } = render(
      <ReleaseNotesProvider
        initReleaseNotes={{ latest: '', latestVersion: 10, past: '' }}
      >
        <ReadReleaseNotesComponent />
      </ReleaseNotesProvider>,
    );

    rerender(
      <ReleaseNotesProvider
        initReleaseNotes={{ latest: '', latestVersion: 10, past: '' }}
      >
        <ShowUnreadReleaseNotesComponent />
      </ReleaseNotesProvider>,
    );

    expect(screen.getByText('no new release notes')).toBeInTheDocument();

    cleanup();
    rerender = render(
      <ReleaseNotesProvider
        initReleaseNotes={{ latest: '', latestVersion: 8, past: '' }}
      >
        <ShowUnreadReleaseNotesComponent />
      </ReleaseNotesProvider>,
    ).rerender;

    expect(screen.getByText('no new release notes')).toBeInTheDocument();

    rerender(
      <ReleaseNotesProvider
        initReleaseNotes={{ latest: '', latestVersion: 8, past: '' }}
      >
        <ShowUnreadReleaseNotesComponent />
      </ReleaseNotesProvider>,
    );
    expect(getLastReadVersion()).toBe(10);
  });
});
