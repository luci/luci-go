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

import { LinearProgress } from '@mui/material';
import { observer } from 'mobx-react-lite';
import { useEffect } from 'react';
import { Outlet, useNavigate, useParams } from 'react-router-dom';

import '../test_results_tab/count_indicator';
import { Tab, Tabs } from '../../components/tabs';
import { INVOCATION_STATE_DISPLAY_MAP } from '../../libs/constants';
import { getBuildURLPathFromBuildId, getInvURLPath, getSwarmingTaskURL } from '../../libs/url_utils';
import { useStore } from '../../store';
import { InvLitEnvProvider } from './inv_lit_env_provider';

// Should be checked upstream, but allowlist URLs here just to be safe.
const ALLOWED_SWARMING_HOSTS = [
  'chromium-swarm-dev.appspot.com',
  'chromium-swarm.appspot.com',
  'chrome-swarming.appspot.com',
];

export const InvocationPage = observer(() => {
  const { invId } = useParams();
  const store = useStore();
  const navigate = useNavigate();

  if (!invId) {
    throw new Error('invariant violated: invId should be set');
  }

  useEffect(() => {
    store.invocationPage.setInvocationId(invId);
    document.title = `inv: ${invId}`;
  }, [invId]);

  const inv = store.invocationPage.invocation.invocation;
  const buildId = invId.match(/^build-(?<id>\d+)/)?.groups?.['id'];
  const { swarmingHost, taskId } = invId.match(/^task-(?<swarmingHost>.*)-(?<taskId>[0-9a-fA-F]+)$/)?.groups || {};
  const invUrlPath = getInvURLPath(invId);

  return (
    <InvLitEnvProvider>
      <div
        css={{
          backgroundColor: 'var(--block-background-color)',
          padding: '6px 16px',
          fontFamily: "'Google Sans', 'Helvetica Neue', sans-serif",
          fontSize: '14px',
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
              <a href={getBuildURLPathFromBuildId(buildId)} target="_blank">
                build page
              </a>
              )
            </>
          )}
          {ALLOWED_SWARMING_HOSTS.includes(swarmingHost) && taskId && (
            <a href={getSwarmingTaskURL(swarmingHost, taskId)} target="_blank">
              task page
            </a>
          )}
        </div>
        <div
          css={{
            marginLeft: 'auto',
            flex: '0 auto',
          }}
        >
          {inv && (
            <>
              <i>{INVOCATION_STATE_DISPLAY_MAP[inv.state]}</i>
              {inv.finalizeTime ? (
                <> at {new Date(inv.finalizeTime).toLocaleString()}</>
              ) : (
                <> since {new Date(inv.createTime).toLocaleString()}</>
              )}
            </>
          )}
        </div>
      </div>
      <LinearProgress value={100} variant={inv ? 'determinate' : 'indeterminate'} />
      <Tabs
        value={store.selectedTabId || false}
        onChange={(_, tabId) => {
          navigate(`${invUrlPath}/${tabId}`);
        }}
      >
        <Tab
          label="Test Results"
          value="test-results"
          href={invUrlPath + '/test-results'}
          icon={<milo-trt-count-indicator />}
          iconPosition="end"
        />
        <Tab label="Invocation Details" value="invocation-details" href={invUrlPath + '/invocation-details'} />
      </Tabs>
      <Outlet />
    </InvLitEnvProvider>
  );
});
