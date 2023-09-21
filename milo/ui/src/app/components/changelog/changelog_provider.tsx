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

import {
  ReactNode,
  createContext,
  useCallback,
  useContext,
  useRef,
  useState,
} from 'react';

import { Changelog, bumpLastReadVersion, getLastReadVersion } from './common';

const ChangelogCtx = createContext<Changelog | null>(null);
const HasNewChangelogCtx = createContext<boolean | null>(null);
const MarkChangelogAsReadCtx = createContext<(() => void) | null>(null);

export interface ChangelogProviderProps {
  readonly initChangelog: Changelog;
  readonly children: ReactNode;
}

export function ChangelogProvider({
  initChangelog,
  children,
}: ChangelogProviderProps) {
  const changelogRef = useRef(initChangelog);
  const [hasNew, setHasNew] = useState(
    // Last read version might be larger after a rollback.
    () => getLastReadVersion() < changelogRef.current.latestVersion,
  );

  const markAsRead = useCallback(() => {
    setHasNew(false);
    bumpLastReadVersion(changelogRef.current.latestVersion);
  }, []);

  return (
    <ChangelogCtx.Provider value={changelogRef.current}>
      <MarkChangelogAsReadCtx.Provider value={markAsRead}>
        <HasNewChangelogCtx.Provider value={hasNew}>
          {children}
        </HasNewChangelogCtx.Provider>
      </MarkChangelogAsReadCtx.Provider>
    </ChangelogCtx.Provider>
  );
}

export function useChangelog() {
  const ctx = useContext(ChangelogCtx);
  if (ctx === null) {
    throw new Error('useChangelog must be used within ChangelogStateProvider');
  }
  return ctx;
}
export function useHasNewChangelog() {
  const ctx = useContext(HasNewChangelogCtx);
  if (ctx === null) {
    throw new Error(
      'useHasNewChangelog must be used within ChangelogStateProvider',
    );
  }
  return ctx;
}

export function useMarkChangelogAsRead() {
  const ctx = useContext(MarkChangelogAsReadCtx);
  if (ctx === null) {
    throw new Error(
      'useMarkChangelogAsRead must be used within ChangelogStateProvider',
    );
  }
  return ctx;
}
