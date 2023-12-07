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

import { Link, TableCell } from '@mui/material';

import { getAssociatedGitilesCommit } from '@/build/tools/build_utils';
import {
  getGitilesCommitLabel,
  getGitilesCommitURL,
} from '@/common/tools/gitiles_utils';

import { useBuild } from './context';

export function CommitHeadCell() {
  return <TableCell width="1px">Commit</TableCell>;
}

export function CommitContentCell() {
  const build = useBuild();
  const commit = getAssociatedGitilesCommit(build);

  return (
    <TableCell>
      {commit ? (
        <Link href={getGitilesCommitURL(commit)}>
          {getGitilesCommitLabel(commit)}
        </Link>
      ) : (
        'N/A'
      )}
    </TableCell>
  );
}
