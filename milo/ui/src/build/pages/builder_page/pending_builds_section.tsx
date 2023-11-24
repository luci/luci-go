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

import { CircularProgress, Link } from '@mui/material';
import { DateTime } from 'luxon';

import { RelativeDurationBadge } from '@/common/components/relative_duration_badge';
import { usePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import {
  BuilderID,
  BuildsService,
  BuildbucketStatus,
} from '@/common/services/buildbucket';
import { getBuildURLPathFromBuildId } from '@/common/tools/url_utils';

const PAGE_SIZE = 100;
const FIELD_MASK = 'builds.*.id,builds.*.number,builds.*.createTime';

export interface PendingBuildsSectionProps {
  readonly builderId: BuilderID;
}

export function PendingBuildsSection({ builderId }: PendingBuildsSectionProps) {
  const { data, error, isError, isLoading } = usePrpcQuery({
    host: SETTINGS.buildbucket.host,
    Service: BuildsService,
    method: 'searchBuilds',
    request: {
      predicate: {
        builder: builderId,
        includeExperimental: true,
        status: BuildbucketStatus.Scheduled,
      },
      pageSize: PAGE_SIZE,
      fields: FIELD_MASK,
    },
  });

  if (isError) {
    throw error;
  }

  return (
    <>
      <h3>
        Scheduled Builds
        {!isLoading && (
          <>
            {' '}
            (
            {data.nextPageToken
              ? `most recent ${PAGE_SIZE} builds`
              : data.builds?.length || 0}
            )
          </>
        )}
      </h3>
      {isLoading ? (
        <CircularProgress />
      ) : (
        <ul css={{ maxHeight: '400px', overflow: 'auto' }}>
          {data.builds?.map((b) => {
            return (
              <li key={b.id}>
                <Link href={getBuildURLPathFromBuildId(b.id)}>
                  {b.number || `b${b.id}`}
                </Link>{' '}
                <RelativeDurationBadge
                  css={{ verticalAlign: 'text-top' }}
                  from={DateTime.fromISO(b.createTime)}
                />{' '}
                ago
              </li>
            );
          })}
        </ul>
      )}
    </>
  );
}
