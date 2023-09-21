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

import {
  ChangelogProvider,
  useHasNewChangelog,
  useMarkChangelogAsRead,
} from './changelog_provider';
import { getLastReadVersion } from './common';

function ReadChangelogComponent() {
  const markAsRead = useMarkChangelogAsRead();
  useEffect(() => markAsRead(), [markAsRead]);
  return <></>;
}

function ShowUnreadChangelogComponent() {
  const hasNew = useHasNewChangelog();
  return <>{hasNew ? 'has new changelog' : 'no new changelog'}</>;
}

describe('ChangelogProvider', () => {
  afterEach(() => {
    cleanup();
    localStorage.clear();
  });

  it('can mark changelog as read', () => {
    const { rerender } = render(
      <ChangelogProvider
        initChangelog={{ latest: '', latestVersion: 10, past: '' }}
      >
        <ShowUnreadChangelogComponent />
      </ChangelogProvider>,
    );
    expect(getLastReadVersion()).toBe(-1);
    expect(screen.getByText('has new changelog')).toBeInTheDocument();

    rerender(
      <ChangelogProvider
        initChangelog={{ latest: '', latestVersion: 10, past: '' }}
      >
        <ReadChangelogComponent />
      </ChangelogProvider>,
    );
    expect(getLastReadVersion()).toBe(10);

    rerender(
      <ChangelogProvider
        initChangelog={{ latest: '', latestVersion: 10, past: '' }}
      >
        <ShowUnreadChangelogComponent />
      </ChangelogProvider>,
    );

    expect(screen.getByText('no new changelog')).toBeInTheDocument();
  });

  it('can handle rollback', () => {
    let { rerender } = render(
      <ChangelogProvider
        initChangelog={{ latest: '', latestVersion: 10, past: '' }}
      >
        <ReadChangelogComponent />
      </ChangelogProvider>,
    );

    rerender(
      <ChangelogProvider
        initChangelog={{ latest: '', latestVersion: 10, past: '' }}
      >
        <ShowUnreadChangelogComponent />
      </ChangelogProvider>,
    );

    expect(screen.getByText('no new changelog')).toBeInTheDocument();

    cleanup();
    rerender = render(
      <ChangelogProvider
        initChangelog={{ latest: '', latestVersion: 8, past: '' }}
      >
        <ShowUnreadChangelogComponent />
      </ChangelogProvider>,
    ).rerender;

    expect(screen.getByText('no new changelog')).toBeInTheDocument();

    rerender(
      <ChangelogProvider
        initChangelog={{ latest: '', latestVersion: 8, past: '' }}
      >
        <ShowUnreadChangelogComponent />
      </ChangelogProvider>,
    );
    expect(getLastReadVersion()).toBe(10);
  });
});
