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

import { Box, Link, Skeleton, styled } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { DateTime } from 'luxon';
import { Link as RouterLink } from 'react-router-dom';

import { BuildStatusIcon } from '@/build/components/build_status_icon';
import {
  BUILD_STATUS_CLASS_MAP,
  BUILD_STATUS_DISPLAY_MAP,
  PARTIAL_BUILD_FIELD_MASK,
} from '@/build/constants';
import { PartialBuild } from '@/build/types';
import { DurationBadge } from '@/common/components/duration_badge';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import { Timestamp } from '@/common/components/timestamp';
import { SHORT_TIME_FORMAT } from '@/common/tools/time_utils';
import {
  getBuildURLPathFromBuildData,
  getBuilderURLPath,
  getProjectURLPath,
} from '@/common/tools/url_utils';
import { GetBuildRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

import { useBuildsClient } from './hooks';

const ChipContainer = styled(Box)`
  display: grid;
  grid-template-columns: auto 1fr;
  line-height: 20px;
  grid-gap: 5px;
  font-size: 1em;
`;

interface TooltipProps {
  readonly build: PartialBuild;
}

function Tooltip({ build }: TooltipProps) {
  return (
    <table>
      <thead>
        <tr>
          <td colSpan={2} css={{ fontWeight: 'bold', paddingBottom: '10px' }}>
            Click to open ancestor build.
          </td>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td>Project:</td>
          <td>
            <Link
              component={RouterLink}
              to={getProjectURLPath(build.builder.project)}
            >
              {build.builder.project}
            </Link>
          </td>
        </tr>
        <tr>
          <td>Bucket:</td>
          <td>{build.builder.bucket}</td>
        </tr>
        <tr>
          <td>Builder:</td>
          <td>
            <Link component={RouterLink} to={getBuilderURLPath(build.builder)}>
              {build.builder.builder}
            </Link>
          </td>
        </tr>
        <tr>
          <td>Build:</td>
          <td>
            <Link
              component={RouterLink}
              to={getBuildURLPathFromBuildData(build)}
            >
              {build.number || 'b' + build.id}
            </Link>
          </td>
        </tr>
        <tr>
          <td>Status:</td>
          <td>
            <BuildStatusIcon status={build.status} />{' '}
            <span className={BUILD_STATUS_CLASS_MAP[build.status]}>
              {BUILD_STATUS_DISPLAY_MAP[build.status]}
            </span>
          </td>
        </tr>
        <tr>
          <td>Create Time:</td>
          <td>
            <Timestamp
              datetime={DateTime.fromISO(build.createTime)}
              format={SHORT_TIME_FORMAT}
            />
          </td>
        </tr>
        <tr>
          <td>Duration:</td>
          <td>
            <DurationBadge
              durationLabel="Execution Duration"
              fromLabel="Start Time"
              from={build.startTime ? DateTime.fromISO(build.startTime) : null}
              toLabel="End Time"
              to={build.endTime ? DateTime.fromISO(build.endTime) : null}
            />
          </td>
        </tr>
      </tbody>
    </table>
  );
}

export interface AncestorBuildProps {
  readonly buildId: string;
}

export function AncestorBuild({ buildId }: AncestorBuildProps) {
  const client = useBuildsClient();
  const { data, isError, error } = useQuery({
    ...client.GetBuild.query(
      GetBuildRequest.fromPartial({
        id: buildId,
        mask: { fields: PARTIAL_BUILD_FIELD_MASK },
      }),
    ),
    select: (data) => data as PartialBuild,
  });
  if (isError) {
    throw error;
  }

  return (
    <HtmlTooltip arrow title={data && <Tooltip build={data} />}>
      <ChipContainer>
        {data ? (
          <BuildStatusIcon status={data.status} fontSize="small" />
        ) : (
          <Skeleton variant="circular" width="20px" height="20px" />
        )}
        {data ? (
          <Link
            component={RouterLink}
            sx={{ fontWeight: 'bold' }}
            to={getBuildURLPathFromBuildData(data)}
          >
            {data.builder.builder}
          </Link>
        ) : (
          <Skeleton width="200px" />
        )}
      </ChipContainer>
    </HtmlTooltip>
  );
}
