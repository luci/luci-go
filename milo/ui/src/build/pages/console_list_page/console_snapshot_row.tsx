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
import { Link as RouterLink } from 'react-router';

import { BUILD_STATUS_CLASS_MAP } from '@/build/constants';
import { formatBuilderId } from '@/build/tools/build_utils';
import { OutputConsoleSnapshot } from '@/build/types';
import {
  getBuildURLPathFromBuildData,
  getBuilderURLPath,
  getOldConsoleURLPath,
} from '@/common/tools/url_utils';
import { extractProject } from '@/common/tools/utils';
import { CategoryTree } from '@/generic_libs/tools/category_tree';

const Row = styled.tr({
  // Shrink-to-fit on all but the last column.
  '& > td:not(:last-of-type)': { width: 1 },
  '& > td': {
    padding: '5px 2px',
  },
});

const BuildersCell = styled.td({
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

export interface ConsoleSnapshotRowProps {
  readonly snapshot: OutputConsoleSnapshot;
}

export function ConsoleSnapshotRow({ snapshot }: ConsoleSnapshotRowProps) {
  const project = extractProject(snapshot.console.realm);

  // Sort builder snapshots by builder categories.
  const sortedBuilderSnapshots = useMemo(() => {
    const entries =
      snapshot.console.builders.map(
        (b, i) =>
          [
            [...(b.category || '').split('|'), formatBuilderId(b.id)],
            // The number of builder snapshots always match the number of builders.

            snapshot.builderSnapshots![i],
          ] as const,
      ) || [];

    // Builders are sorted in a weird way. Items (could be builders,
    // subcategories, or a mix of both) under the same category are closely
    // located. Within each category, items are sorted by the order they are
    // first discovered (when iterating over Console.builders). We need to keep
    // this behavior because some users rely on this order to quickly tell the
    // builder each cell represents (see crbug.com/1495253).
    const tree = new CategoryTree(entries);
    return [...tree.values()];
  }, [snapshot]);

  return (
    <Row>
      <td>
        <Link
          href={getOldConsoleURLPath(project, snapshot.console.id)}
          data-testid="console-url"
        >
          <b>{snapshot.console.name}</b>
        </Link>
      </td>
      <td>
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
      </td>
      <BuildersCell data-testid="builder-snapshots">
        {sortedBuilderSnapshots.map((s) => {
          const builderId = formatBuilderId(s.builder);
          const link = s.build
            ? getBuildURLPathFromBuildData(s.build)
            : getBuilderURLPath(s.builder);
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
      </BuildersCell>
    </Row>
  );
}
