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

import { Link, Skeleton, TableCell } from '@mui/material';

import { useCommit, useRepoUrl } from './context';

export function IdHeadCell() {
  return <TableCell width="1px">ID</TableCell>;
}

export function IdContentCell() {
  const repoUrl = useRepoUrl();
  const commit = useCommit();

  return (
    <TableCell sx={{ minWidth: '65px' }}>
      {commit ? (
        <Link
          href={`${repoUrl}/+/${commit.id}`}
          target="_blank"
          rel="noreferrer"
          sx={{ fontWeight: 'bold' }}
        >
          {commit.id.substring(0, 8)}
        </Link>
      ) : (
        <Skeleton />
      )}
    </TableCell>
  );
}
