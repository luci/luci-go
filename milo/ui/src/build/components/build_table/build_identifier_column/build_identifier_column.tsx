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

import { Box, Chip, Link, TableCell } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';

import { BuildStatusIcon } from '@/build/components/build_status_icon';
import {
  getBuildURLPathFromBuildData,
  getBuilderURLPath,
  getProjectURLPath,
} from '@/common/tools/url_utils';

import { useBuild } from '../context';

export function BuildIdentifierHeadCell() {
  return <TableCell width="1px">Build</TableCell>;
}

export interface BuildIdentifierContentCellProps {
  /**
   * The default project of the table.
   *
   * If the build is in the default project, the project identifier is omitted.
   */
  readonly defaultProject?: string;
  /**
   * The indentation level of the build identifier.
   *
   * Defaults to `0`.
   */
  readonly indentLevel?: number;
  /**
   * Whether the build is the currently rendered build (e.g. in a build page).
   *
   * If true, the build will be marked with a "this build" chip.
   * Defaults to `false`.
   */
  readonly selected?: boolean;
}

export function BuildIdentifierContentCell({
  defaultProject,
  indentLevel = 0,
  selected = false,
}: BuildIdentifierContentCellProps) {
  const build = useBuild();

  return (
    <TableCell>
      <Box
        sx={{
          display: 'inline-grid',
          gridTemplateColumns: `repeat(${indentLevel}, 30px)`,
        }}
      >
        {Array(indentLevel)
          .fill(0)
          .map((_, i) => (
            <Box key={i} />
          ))}
      </Box>
      <BuildStatusIcon status={build.status} sx={{ marginRight: '5px' }} />
      <Box
        sx={{
          display: 'inline-block',
          fontWeight: selected ? 'bold' : 'unset',
        }}
      >
        {defaultProject !== build.builder.project && (
          <>
            <Link
              component={RouterLink}
              to={getProjectURLPath(build.builder.project)}
            >
              {build.builder.project}
            </Link>
            /
          </>
        )}
        {build.builder.bucket}/
        <Link component={RouterLink} to={getBuilderURLPath(build.builder)}>
          {build.builder.builder}
        </Link>
        /
        {selected ? (
          <>{build.number ? build.number : 'b' + build.id}</>
        ) : (
          <Link component={RouterLink} to={getBuildURLPathFromBuildData(build)}>
            {build.number ? build.number : 'b' + build.id}
          </Link>
        )}
      </Box>
      {selected && (
        <Chip
          label="this build"
          size="small"
          color="primary"
          title="this is the build you are currently viewing"
          sx={{ marginLeft: '4px' }}
        />
      )}
    </TableCell>
  );
}
