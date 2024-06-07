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

import { Box, Link, styled } from '@mui/material';
import { DateTime } from 'luxon';
import { Link as RouterLink } from 'react-router-dom';

import { Timestamp } from '@/common/components/timestamp';
import { SHORT_TIME_FORMAT } from '@/common/tools/time_utils';
import { getBuilderURLPath, getProjectURLPath } from '@/common/tools/url_utils';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';

import { useBuild } from './context';
import { CustomBugLink } from './custom_bug_link';

const Delimiter = styled(Box)({
  borderLeft: '1px solid var(--divider-color)',
  width: '1px',
  marginLeft: '10px',
  marginRight: '10px',
  '& + &': {
    display: 'none',
  },
});

export interface BuildIdBarProps {
  readonly builderId: BuilderID;
  readonly buildNumOrId: string;
}

export function BuildIdBar({ builderId, buildNumOrId }: BuildIdBarProps) {
  const build = useBuild();

  return (
    <div
      css={{
        backgroundColor: 'var(--block-background-color)',
        padding: '6px 16px',
        display: 'flex',
      }}
    >
      <div css={{ flex: '0 auto' }}>
        <span css={{ color: 'var(--light-text-color)' }}>Build </span>
        <Link component={RouterLink} to={getProjectURLPath(builderId.project)}>
          {builderId.project}
        </Link>
        <span> / </span>
        <span>{builderId.bucket}</span>
        <span> / </span>
        <Link component={RouterLink} to={getBuilderURLPath(builderId)}>
          {builderId.builder}
        </Link>
        <span> / </span>
        <span>{buildNumOrId}</span>
      </div>
      <Delimiter />
      <CustomBugLink project={builderId.project} build={build} />
      <Delimiter />
      {/* TODO: info that helps users identify the build (e.g. source)
       ** should be displayed at the top (above the tabs). At the moment
       ** the source description is not short enough to be put here.
       ** Also we may want to redesign this bar to give more emphasize to
       ** the build identifier (builder, timestamp, and source).
       */}
      {build?.createTime ? (
        <span>
          created at{' '}
          <Timestamp
            datetime={DateTime.fromISO(build.createTime)}
            format={SHORT_TIME_FORMAT}
          />
        </span>
      ) : null}
    </div>
  );
}
