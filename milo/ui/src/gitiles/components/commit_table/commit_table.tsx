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

import { Table } from '@mui/material';
import { ReactNode, useEffect, useRef, useState } from 'react';
import { useLatest } from 'react-use';

import {
  DefaultExpandedProvider,
  RepoUrlProvider,
  SetDefaultExpandedProvider,
} from './context';

export interface CommitTableProps {
  readonly repoUrl: string;
  readonly initDefaultExpanded?: boolean;
  readonly onDefaultExpandedChanged?: (expand: boolean) => void;
  readonly children: ReactNode;
}

export function CommitTable({
  repoUrl,
  initDefaultExpanded = false,
  onDefaultExpandedChanged = () => {
    /* Noop by default. */
  },
  children,
}: CommitTableProps) {
  const [defaultExpanded, setDefaultExpanded] = useState(initDefaultExpanded);

  const onDefaultExpandedChangedRef = useLatest(onDefaultExpandedChanged);
  const isFirstCall = useRef(true);
  useEffect(() => {
    // Skip the first call because the default state were not changed.
    if (isFirstCall.current) {
      isFirstCall.current = false;
      return;
    }
    onDefaultExpandedChangedRef.current(defaultExpanded);
  }, [onDefaultExpandedChangedRef, defaultExpanded]);

  return (
    <Table
      size="small"
      sx={{
        borderCollapse: 'separate',
        '& td, th': {
          padding: '0px 8px',
        },
        minWidth: '1000px',
      }}
    >
      <SetDefaultExpandedProvider value={setDefaultExpanded}>
        <DefaultExpandedProvider value={defaultExpanded}>
          <RepoUrlProvider value={repoUrl}>{children}</RepoUrlProvider>
        </DefaultExpandedProvider>
      </SetDefaultExpandedProvider>
    </Table>
  );
}
