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

import {
  ReleaseNotes,
  bumpLastReadVersion,
  getLastReadVersion,
} from './common';

const ReleaseNotesCtx = createContext<ReleaseNotes | null>(null);
const HasNewReleaseCtx = createContext<boolean | null>(null);
const MarkReleaseNotesAsReadCtx = createContext<(() => void) | null>(null);

export interface ReleaseNotesProviderProps {
  readonly initReleaseNotes: ReleaseNotes;
  readonly children: ReactNode;
}

export function ReleaseNotesProvider({
  initReleaseNotes,
  children,
}: ReleaseNotesProviderProps) {
  const releaseNotesRef = useRef(initReleaseNotes);
  const [hasNew, setHasNew] = useState(
    // Last read version might be larger after a rollback.
    () => getLastReadVersion() < releaseNotesRef.current.latestVersion,
  );

  const markAsRead = useCallback(() => {
    setHasNew(false);
    bumpLastReadVersion(releaseNotesRef.current.latestVersion);
  }, []);

  return (
    <ReleaseNotesCtx.Provider value={releaseNotesRef.current}>
      <MarkReleaseNotesAsReadCtx.Provider value={markAsRead}>
        <HasNewReleaseCtx.Provider value={hasNew}>
          {children}
        </HasNewReleaseCtx.Provider>
      </MarkReleaseNotesAsReadCtx.Provider>
    </ReleaseNotesCtx.Provider>
  );
}

export function useReleaseNotes() {
  const ctx = useContext(ReleaseNotesCtx);
  if (ctx === null) {
    throw new Error('useReleaseNotes must be used within ReleaseNotesProvider');
  }
  return ctx;
}
export function useHasNewRelease() {
  const ctx = useContext(HasNewReleaseCtx);
  if (ctx === null) {
    throw new Error(
      'useHasNewRelease must be used within ReleaseNotesProvider',
    );
  }
  return ctx;
}

export function useMarkReleaseNotesRead() {
  const ctx = useContext(MarkReleaseNotesAsReadCtx);
  if (ctx === null) {
    throw new Error(
      'useMarkReleaseNotesRead must be used within ReleaseNotesProvider',
    );
  }
  return ctx;
}
