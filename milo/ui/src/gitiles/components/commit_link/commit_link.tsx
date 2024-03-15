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

import { HtmlTooltip } from '@/common/components/html_tooltip';
import {
  getGitilesCommitLabel,
  getGitilesCommitURL,
} from '@/gitiles/tools/utils';
import { GitilesCommit } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { CommitLinkTooltip } from './commit_link_tooltip';

export interface CommitLinkProps {
  readonly commit: GitilesCommit;
}

export function CommitLink({ commit }: CommitLinkProps) {
  return (
    <HtmlTooltip title={<CommitLinkTooltip commit={commit} />}>
      <Link href={getGitilesCommitURL(commit)}>
        {getGitilesCommitLabel(commit)}
      </Link>
    </HtmlTooltip>
  );
}
