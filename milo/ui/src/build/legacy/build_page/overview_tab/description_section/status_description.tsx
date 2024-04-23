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

import { DateTime } from 'luxon';

import {
  BUILD_STATUS_CLASS_MAP,
  BUILD_STATUS_DISPLAY_MAP,
} from '@/build/constants';
import { SpecifiedBuildStatus } from '@/build/types';
import { DurationBadge } from '@/common/components/duration_badge';
import { Timestamp } from '@/common/components/timestamp';
import { SHORT_TIME_FORMAT } from '@/common/tools/time_utils';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

export interface StatusDescriptionProps {
  readonly build: Build;
}

export function StatusDescription({ build }: StatusDescriptionProps) {
  const createTime = DateTime.fromISO(build.createTime!);

  const status = build.status as SpecifiedBuildStatus;
  const startTime = build.startTime ? DateTime.fromISO(build.startTime) : null;
  const endTime = build.endTime ? DateTime.fromISO(build.endTime) : null;

  const statusDisplay = (
    <b className={BUILD_STATUS_CLASS_MAP[status]}>
      {BUILD_STATUS_DISPLAY_MAP[status]}
    </b>
  );
  const durationDisplay =
    startTime && endTime ? (
      <>
        {' '}
        <DurationBadge
          duration={endTime.diff(startTime)}
          from={startTime}
          to={endTime}
          css={{ verticalAlign: 'middle' }}
        />{' '}
        after it started
      </>
    ) : (
      <></>
    );

  if (status === Status.SCHEDULED) {
    return (
      <>
        was {statusDisplay} at <Timestamp datetime={createTime} />
      </>
    );
  }

  if (status === Status.STARTED) {
    return (
      <>
        is {statusDisplay} since{' '}
        <Timestamp datetime={startTime!} format={SHORT_TIME_FORMAT} />
      </>
    );
  }

  if (status === Status.CANCELED) {
    return (
      <>
        was {statusDisplay} by {build.canceledBy || '<unknown>'} at{' '}
        <Timestamp datetime={endTime!} format={SHORT_TIME_FORMAT} />
        {durationDisplay}
      </>
    );
  }

  return (
    <>
      {statusDisplay} at{' '}
      <Timestamp datetime={endTime!} format={SHORT_TIME_FORMAT} />
      {durationDisplay}
    </>
  );
}
