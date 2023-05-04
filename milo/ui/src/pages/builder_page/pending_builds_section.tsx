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

import { Timestamp } from '../../components/timestamp';
import { displayCompactDuration, NUMERIC_TIME_FORMAT } from '../../libs/time_utils';
import { getBuildURLPathFromBuildId } from '../../libs/url_utils';
import { BuilderID, BuildStatus } from '../../services/buildbucket';
import { useBuilds } from './utils';

const PAGE_SIZE = 100;
const FIELD_MASK = 'builds.*.id,builds.*.number,builds.*.createTime';

export interface PendingBuildsSectionProps {
  readonly builderId: BuilderID;
}

export function PendingBuildsSection({ builderId }: PendingBuildsSectionProps) {
  const { data, error, isError, isLoading } = useBuilds({
    predicate: { builder: builderId, includeExperimental: true, status: BuildStatus.Scheduled },
    pageSize: PAGE_SIZE,
    fields: FIELD_MASK,
  });

  if (isError) {
    throw error;
  }

  const now = DateTime.now();

  return (
    <>
      <h3>
        Scheduled Builds
        {!isLoading && <> ({data.nextPageToken ? `most recent ${PAGE_SIZE} builds` : data.builds?.length || 0})</>}
      </h3>
      {isLoading ? (
        <CircularProgress />
      ) : (
        <>
          <ul>
            {data.builds?.map((b) => {
              const createTime = DateTime.fromISO(b.createTime);
              const [duration] = displayCompactDuration(now.diff(createTime));
              return (
                <li key={b.id}>
                  <Link href={getBuildURLPathFromBuildId(b.id)}>{b.number || `b${b.id}`}</Link>
                  <span> </span>
                  Created at: <Timestamp datetime={createTime} format={NUMERIC_TIME_FORMAT} />, waiting {duration}
                </li>
              );
            })}
          </ul>
        </>
      )}
    </>
  );
}
