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

import { Link } from '@mui/material';

import { getGitilesRepoURL } from '@/gitiles/tools/utils';
import { GitilesCommit } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { Commit } from '@/proto/go.chromium.org/luci/common/proto/git/commit.pb';

export interface RevisionRangeProps {
  readonly blamelistPin: GitilesCommit;
  readonly commitCount: number;
  readonly precedingCommit?: Commit;
}

export function RevisionRange({
  blamelistPin,
  commitCount,
  precedingCommit,
}: RevisionRangeProps) {
  if (!commitCount) {
    return <></>;
  }

  if (precedingCommit) {
    const repoUrl = getGitilesRepoURL(blamelistPin);
    const revisionRange =
      precedingCommit.id.substring(0, 8) + '..' + blamelistPin.id!.slice(0, 8);
    return (
      <>
        This build included {commitCount} new revisions from{' '}
        <Link
          href={`${repoUrl}/+log/${revisionRange}`}
          target="_blank"
          rel="noreferrer"
        >
          {revisionRange}
        </Link>
      </>
    );
  }

  return (
    <>
      This build included over {commitCount} new revisions up to{' '}
      {blamelistPin.id?.slice(0, 8)}
    </>
  );
}
