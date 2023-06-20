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

import { usePrpcQuery } from '@/common/hooks/use_prpc_query';
import {
  BuilderID,
  BuildsService,
  BuildStatus,
} from '@/common/services/buildbucket';
import { displayCompactDuration } from '@/common/tools/time_utils';
import { getBuildURLPathFromBuildId } from '@/common/tools/url_utils';

const PAGE_SIZE = 100;
const FIELD_MASK = 'builds.*.id,builds.*.number,builds.*.startTime';

export interface StartedBuildsSectionProps {
  readonly builderId: BuilderID;
}

export function StartedBuildsSection({ builderId }: StartedBuildsSectionProps) {
  const { data, error, isError, isLoading } = usePrpcQuery({
    host: CONFIGS.BUILDBUCKET.HOST,
    Service: BuildsService,
    method: 'searchBuilds',
    request: {
      predicate: {
        builder: builderId,
        includeExperimental: true,
        status: BuildStatus.Started,
      },
      pageSize: PAGE_SIZE,
      fields: FIELD_MASK,
    },
  });

  if (isError) {
    throw error;
  }

  const now = DateTime.now();

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
        <ul>
          {data.builds?.map((b) => {
            const [duration] = displayCompactDuration(
              now.diff(DateTime.fromISO(b.startTime!))
            );
            return (
              <li key={b.id}>
                <Link href={getBuildURLPathFromBuildId(b.id)}>
                  {b.number || `b${b.id}`}
                </Link>
                <span> </span>
                [Running for: {duration}]
              </li>
            );
          })}
        </ul>
      )}
    </>
  );
}
