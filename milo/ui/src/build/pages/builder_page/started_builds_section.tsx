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
import { useQuery } from '@tanstack/react-query';
import { DateTime } from 'luxon';

import { useBuildsClient } from '@/build/hooks/prpc_clients';
import { DurationBadge } from '@/common/components/duration_badge';
import { getBuildURLPathFromBuildId } from '@/common/tools/url_utils';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { SearchBuildsRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

const PAGE_SIZE = 100;
const FIELD_MASK = Object.freeze([
  'builds.*.id',
  'builds.*.number',
  'builds.*.start_time',
]);

export interface StartedBuildsSectionProps {
  readonly builderId: BuilderID;
}

export function StartedBuildsSection({ builderId }: StartedBuildsSectionProps) {
  const client = useBuildsClient();
  const { data, error, isError, isLoading } = useQuery(
    client.SearchBuilds.query(
      SearchBuildsRequest.fromPartial({
        predicate: {
          builder: builderId,
          includeExperimental: true,
          status: Status.STARTED,
        },
        pageSize: PAGE_SIZE,
        fields: FIELD_MASK,
      }),
    ),
  );

  if (isError) {
    throw error;
  }

  return (
    <>
      <h3>
        Started Builds
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
                <DurationBadge
                  durationLabel="Execution Duration"
                  fromLabel="Start Time"
                  from={DateTime.fromISO(b.startTime!)}
                  toLabel="End Time"
                  intervalMs={10_000}
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
