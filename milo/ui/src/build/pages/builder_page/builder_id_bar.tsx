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

import styled from '@emotion/styled';
import Link from '@mui/material/Link';
import { Link as RouterLink } from 'react-router-dom';

import { BuilderID, HealthStatus } from '@/common/services/buildbucket';
import { getProjectURLPath } from '@/common/tools/url_utils';

import { BuilderHealthIndicator } from './builder_health_indicator';

const Divider = styled.div({
  borderLeft: '1px solid var(--divider-color)',
  width: '1px',
  marginLeft: '10px',
  marginRight: '10px',
});

export interface BuilderIdBarProps {
  readonly builderId: BuilderID;
  readonly healthStatus?: HealthStatus;
}

export function BuilderIdBar({ builderId, healthStatus }: BuilderIdBarProps) {
  return (
    <div
      css={{
        backgroundColor: 'var(--block-background-color)',
        padding: '6px 16px',
        display: 'flex',
      }}
    >
      <div css={{ flex: '0 auto' }}>
        <span css={{ color: 'var(--light-text-color)' }}>Builder </span>
        <Link component={RouterLink} to={getProjectURLPath(builderId.project)}>
          {builderId.project}
        </Link>
        <span> / </span>
        <span>{builderId.bucket}</span>
        <span> / </span>
        <span>{builderId.builder}</span>
      </div>
      {healthStatus && (
        <>
          <Divider />
          <BuilderHealthIndicator healthStatus={healthStatus} />
        </>
      )}
    </div>
  );
}
