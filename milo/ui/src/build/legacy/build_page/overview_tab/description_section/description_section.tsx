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

import { Icon } from '@mui/material';
import { startCase } from 'lodash-es';

import {
  BUILD_STATUS_CLASS_MAP,
  BUILD_STATUS_DISPLAY_MAP,
} from '@/build/constants';
import { SpecifiedBuildStatus } from '@/build/types';
import { BUILD_STATUS_ICON_MAP } from '@/common/constants/build';

import { useBuild } from '../../context';

import { SourceDescription } from './source_description';
import { StatusDescription } from './status_description';

export function BuildDescription() {
  const build = useBuild();
  if (!build) {
    return <></>;
  }

  const input = build.input;
  const output = build.output;
  const status = build.status as SpecifiedBuildStatus;

  const commit = output?.gitilesCommit || input?.gitilesCommit;
  const changes = input?.gerritChanges || [];

  return (
    <>
      <h3>
        <Icon
          className={BUILD_STATUS_CLASS_MAP[status]}
          sx={{ verticalAlign: 'middle' }}
        >
          {BUILD_STATUS_ICON_MAP[status]}
        </Icon>{' '}
        {startCase(BUILD_STATUS_DISPLAY_MAP[status])}
      </h3>
      build{' '}
      {commit ? (
        <>
          <SourceDescription commit={commit} changes={changes} />{' '}
        </>
      ) : (
        ''
      )}
      <StatusDescription build={build} />.
    </>
  );
}
