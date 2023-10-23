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

import styled from '@emotion/styled';
import { Link } from '@mui/material';
import { useMemo } from 'react';
import { Link as RouterLink } from 'react-router-dom';

import { formatBuilderId } from '@/build/tools/utils';
import { BUILD_STATUS_CLASS_MAP } from '@/common/constants';
import { ConsoleSnapshot as SnapshotData } from '@/common/services/milo_internal';
import {
  getBuildURLPathFromBuildData,
  getOldBuilderURLPath,
  getOldConsoleURLPath,
} from '@/common/tools/url_utils';
import { extractProject } from '@/common/tools/utils';

const Container = styled.div({
  display: 'grid',
  alignItems: 'center',
  gridTemplateColumns: 'auto auto 1fr',
  gridColumnGap: '5px',
  margin: '10px 20px',
});

const BuildersContainer = styled.div({
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fit, minmax(2em, 1fr))',
  gap: '1px',
  '& > *': {
    height: '20px',
    borderRadius: '3px',
    border: '1px solid black',
  },
  '& > .no-build-cell': {
    backgroundColor: 'white',
  },
  // The same color-scheme as the builders page.
  // TODO: extract the color pattern to a theme or a common CSS file.
  '& > .pending-cell': {
    backgroundColor: '#ccc',
  },
  '& > .running-cell': {
    backgroundColor: '#fd3',
  },
  '& > .success-cell': {
    backgroundColor: '#8d4',
  },
  '& > .failure-cell': {
    backgroundColor: '#e88',
  },
  '& > .infra-failure-cell': {
    backgroundColor: '#c6c',
  },
  '& > .canceled-cell': {
    backgroundColor: '#8ef',
  },
});

export interface ConsoleSnapshotProps {
  readonly snapshot: SnapshotData;
}

export function ConsoleSnapshot({ snapshot }: ConsoleSnapshotProps) {
  const project = extractProject(snapshot.console.realm);

  // Sort builder snapshots by builder categories.
  const sortedBuilderSnapshots = useMemo(() => {
    const items =
      snapshot.console.builders?.map(
        (b, i) =>
          [
            // Replace the divider with the largest UTF-8 character so sorting
            // the builders by category has the same effect as building a
            // category tree then traversing the leaves.
            (b.category || '').replace('|', '\uFFFF'),
            // The number of builder snapshots always match the number of
            // builders.
            // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
            snapshot.builderSnapshots![i],
          ] as const,
      ) || [];
    return items
      .sort(([cat1], [cat2]) => cat1.localeCompare(cat2))
      .map(([_, snapshot]) => snapshot);
  }, [snapshot]);

  return (
    <Container>
      <Link
        href={getOldConsoleURLPath(project, snapshot.console.id)}
        data-testid="console-url"
      >
        <b>{snapshot.console.name}</b>
      </Link>
      <div>
        (
        <Link
          href={snapshot.console.repoUrl}
          target="_blank"
          rel="noreferrer"
          data-testid="repo-url"
        >
          repo
        </Link>
        )
      </div>
      <BuildersContainer data-testid="builder-snapshots">
        {sortedBuilderSnapshots.map((s) => {
          const builderId = formatBuilderId(s.builder);
          const link = s.build
            ? getBuildURLPathFromBuildData(s.build)
            : getOldBuilderURLPath(s.builder);
          const className = s.build
            ? BUILD_STATUS_CLASS_MAP[s.build.status] + '-cell'
            : 'no-build-cell';

          return (
            <Link
              key={builderId}
              component={RouterLink}
              to={link}
              title={builderId}
              className={className}
            />
          );
        })}
      </BuildersContainer>
    </Container>
  );
}
