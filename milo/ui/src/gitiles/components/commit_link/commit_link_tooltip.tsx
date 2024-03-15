// Copyright 2024 The LUCI Authors.
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

import { Link } from '@mui/material';

import {
  getGitilesCommitURL,
  getGitilesHostURL,
  getGitilesRepoURL,
} from '@/gitiles/tools/utils';
import { GitilesCommit } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

export interface CommitLinkTooltipProps {
  readonly commit: GitilesCommit;
}

export function CommitLinkTooltip({ commit }: CommitLinkTooltipProps) {
  return (
    <table>
      <tbody>
        <tr>
          <td>Host:</td>
          <td>
            <Link href={getGitilesHostURL(commit)}>{commit.host}</Link>
          </td>
        </tr>
        <tr>
          <td>Project:</td>
          <td>
            <Link href={getGitilesRepoURL(commit)}>{commit.project}</Link>
          </td>
        </tr>
        <tr>
          <td>ID:</td>
          <td>
            <Link href={getGitilesCommitURL(commit)}>{commit.id}</Link>
          </td>
        </tr>
        {commit.ref ? (
          <tr>
            <td>Ref:</td>
            <td>{commit.ref}</td>
          </tr>
        ) : (
          <></>
        )}
        {commit.position ? (
          <tr>
            <td>Position:</td>
            <td>{commit.position}</td>
          </tr>
        ) : (
          <></>
        )}
      </tbody>
    </table>
  );
}
