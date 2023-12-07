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

import { Link } from '@mui/material';

import { useAuthState } from '@/common/components/auth_state_provider';
import { HealthStatus } from '@/common/services/buildbucket';

interface BuilderHealthIndicatorProps {
  readonly healthStatus?: HealthStatus;
}

const SCORE_COLOR_MAP: { [score: string]: string } = Object.freeze({
  '0': 'var(--greyed-out-text-color)',
  '1': 'var(--failure-color)',
  ...Object.fromEntries(
    Array(8)
      .fill(0)
      .map((_, i) => [`${i + 2}`, 'var(--warning-color)'])
  ),
  '10': 'var(--success-color)',
});

const TEXT_MAP: { [key: string]: string } = Object.freeze({
  '0': 'Not set',
  '1': 'Low Value',
  ...Object.fromEntries(
    Array(3)
      .fill(0)
      .map((_, i) => [`${i + 2}`, 'Very unhealthy'])
  ),
  '5': 'Unhealthy',
  ...Object.fromEntries(
    Array(4)
      .fill(0)
      .map((_, i) => [`${i + 6}`, 'Slightly unhealthy'])
  ),
  '10': 'Healthy',
});

export function BuilderHealthIndicator({
  healthStatus,
}: BuilderHealthIndicatorProps) {
  const authState = useAuthState();

  if (!healthStatus) {
    return <></>;
  }

  return (
    <>
      <div
        css={{
          display: 'flex',
          flex: '0 auto',
          position: 'relative', // Machine Pool has padding that obscures the bottom half of the link without this
        }}
      >
        <div id="health-indicator">
          <span css={{ color: 'var(--light-text-color)' }}>Health </span>
          <Link href={linkUrl(healthStatus.dataLinks, authState.email)}>
            <span
              id="health-text"
              style={{
                color:
                  SCORE_COLOR_MAP[healthStatus.healthScore || '0'] ||
                  '--critical-failure-color',
              }}
              title={healthStatus.description || ''}
            >
              {TEXT_MAP[healthStatus.healthScore || '0'] ||
                `Unknown health ${healthStatus.healthScore}`}
            </span>
          </Link>
        </div>
      </div>
    </>
  );
}

function linkUrl(
  dataLinks?: { [key: string]: string },
  authEmail?: string,
): string {
  if (!dataLinks) {
    return '';
  }
  const userDomain = authEmail?.split('@').at(-1) || '';
  return dataLinks[userDomain] || dataLinks[''] || '';
}
