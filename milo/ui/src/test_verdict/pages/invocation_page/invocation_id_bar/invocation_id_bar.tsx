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

import { Link } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { DateTime } from 'luxon';
import { Link as RouterLink } from 'react-router-dom';

import { Timestamp } from '@/common/components/timestamp';
import {
  getBuildURLPathFromBuildId,
  getSwarmingTaskURL,
} from '@/common/tools/url_utils';
import { INVOCATION_STATE_DISPLAY_MAP } from '@/test_verdict/constants/invocation';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';

// Should be checked upstream, but allowlist URLs here just to be safe.
const ALLOWED_SWARMING_HOSTS = [
  'chromium-swarm-dev.appspot.com',
  'chromium-swarm.appspot.com',
  'chrome-swarming.appspot.com',
];

export interface InvocationIdBarProps {
  readonly invName: string;
}

export function InvocationIdBar({ invName }: InvocationIdBarProps) {
  const client = useResultDbClient();
  const {
    data: invocation,
    error,
    isError,
  } = useQuery(
    client.GetInvocation.query({
      name: invName,
    }),
  );
  if (isError) {
    throw error;
  }

  const invId = invName.slice('invocations/'.length);
  const buildId = invId.match(/^build-(?<id>\d+)/)?.groups?.['id'];
  const { swarmingHost, taskId } =
    invId.match(/^task-(?<swarmingHost>.*)-(?<taskId>[0-9a-fA-F]+)$/)?.groups ||
    {};

  return (
    <div
      css={{
        backgroundColor: 'var(--block-background-color)',
        padding: '6px 16px',
        display: 'flex',
      }}
    >
      <div css={{ flex: '0 auto' }}>
        <span css={{ color: 'var(--light-text-color)' }}>Invocation ID </span>
        <span>{invId}</span>
        {buildId && (
          <>
            {' '}
            (
            <Link
              component={RouterLink}
              to={getBuildURLPathFromBuildId(buildId)}
              target="_blank"
              rel="noreferrer"
            >
              build
            </Link>
            )
          </>
        )}
        {ALLOWED_SWARMING_HOSTS.includes(swarmingHost) && taskId && (
          <Link
            href={getSwarmingTaskURL(swarmingHost, taskId)}
            target="_blank"
            rel="noreferrer"
          >
            task
          </Link>
        )}
      </div>
      <div
        css={{
          marginLeft: 'auto',
          flex: '0 auto',
        }}
      >
        {invocation && (
          <>
            <i>{INVOCATION_STATE_DISPLAY_MAP[invocation.state]}</i>
            {invocation.finalizeTime ? (
              <>
                {' '}
                at{' '}
                <Timestamp
                  datetime={DateTime.fromISO(invocation.finalizeTime)}
                />
              </>
            ) : (
              <>
                {' '}
                since{' '}
                <Timestamp
                  datetime={DateTime.fromISO(invocation.createTime!)}
                />
              </>
            )}
          </>
        )}
      </div>
    </div>
  );
}
