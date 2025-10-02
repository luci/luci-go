// Copyright 2025 The LUCI Authors.
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

import CodeIcon from '@mui/icons-material/Code';
import CommitIcon from '@mui/icons-material/Commit';
import { Link } from '@mui/material';
import { DateTime } from 'luxon';
import { useMemo } from 'react';

import {
  PageSummaryLine,
  SummaryLineItem,
} from '@/common/components/page_summary_line';
import { PageTitle } from '@/common/components/page_title';
import { Timestamp } from '@/common/components/timestamp';
import { Sources } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';
import {
  formatAllCLs,
  getBuildId,
  getCommitGitilesUrlFromInvocation,
  getCommitInfoFromInvocation,
  getSourcesFromInvocation,
} from '@/test_investigation/utils/test_info_utils';

interface InvocationHeaderProps {
  invocation: AnyInvocation;
}

export function InvocationHeader({ invocation }: InvocationHeaderProps) {
  const displayInvocationId = invocation.name.substring(
    invocation.name.indexOf('/') + 1,
  );
  const buildbucketId = getBuildId(invocation);

  const commitInfo = getCommitInfoFromInvocation(invocation);
  const commitLink = getCommitGitilesUrlFromInvocation(invocation);

  const sources: Sources | undefined | null = useMemo(() => {
    if (!invocation) {
      return undefined;
    }
    return getSourcesFromInvocation(invocation);
  }, [invocation]);

  const cls = formatAllCLs(sources?.changelists);

  return (
    <>
      <PageTitle viewName="Invocation" resourceName={displayInvocationId} />
      <PageSummaryLine>
        {buildbucketId && (
          <SummaryLineItem label="Buildbucket Build">
            <Link
              href={`/ui/b/${buildbucketId}`}
              target="_blank"
              rel="noopener noreferrer"
            >
              {buildbucketId}
            </Link>
          </SummaryLineItem>
        )}
        <SummaryLineItem label="Created">
          <Timestamp datetime={DateTime.fromISO(invocation.createTime || '')} />
        </SummaryLineItem>
        <SummaryLineItem label="Commit" icon={<CommitIcon />}>
          <Link href={commitLink} target="_blank" rel="noopener noreferrer">
            {commitInfo}
          </Link>
        </SummaryLineItem>
        {cls.length > 0 && (
          <SummaryLineItem label="CL" icon={<CodeIcon />}>
            <Link href={cls[0].url} target="_blank" rel="noopener noreferrer">
              {cls[0].display}
            </Link>
            {cls.length > 1 && <> + {cls.length - 1} more</>}
          </SummaryLineItem>
        )}
      </PageSummaryLine>
    </>
  );
}
