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

import { Skeleton, TableCell } from '@mui/material';

import { useCommit } from './context';

export function AuthorHeadCell() {
  return <TableCell width="1px">Author</TableCell>;
}

export function AuthorContentCell() {
  const commit = useCommit();

  return (
    <TableCell
      sx={{
        // Note: `width: '200px'` does not work with `<td />`.
        minWidth: '200px',
        maxWidth: '200px',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
      }}
      title={commit?.author.email}
    >
      {commit ? commit?.author.email : <Skeleton />}
    </TableCell>
  );
}
