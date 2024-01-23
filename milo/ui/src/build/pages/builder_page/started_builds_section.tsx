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

import { RelativeDurationBadge } from '@/common/components/relative_duration_badge';
import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { getBuildURLPathFromBuildId } from '@/common/tools/url_utils';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import {
  BuildsClientImpl,
  SearchBuildsRequest,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
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
  const client = usePrpcServiceClient({
    host: SETTINGS.buildbucket.host,
    ClientImpl: BuildsClientImpl,
  });
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
                <RelativeDurationBadge
                  css={{ verticalAlign: 'text-top' }}
                  // Started builds always have a start time.
                  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
                  from={DateTime.fromISO(b.startTime!)}
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
