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
import { useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta, useProject } from '@/common/components/page_meta';
import { AppRoutedTab, AppRoutedTabs } from '@/common/components/routed_tabs';
import { INVOCATION_STATE_DISPLAY_MAP } from '@/common/constants/legacy';
import { useStore } from '@/common/store';
import { parseInvId } from '@/common/tools/invocation_utils';
import {
  getBuildURLPathFromBuildId,
  getSwarmingTaskURL,
} from '@/common/tools/url_utils';
import { ContentGroup } from '@/generic_libs/components/google_analytics';

import { CountIndicator } from '../test_results_tab/count_indicator';

import { InvLitEnvProvider } from './inv_lit_env_provider';

// Should be checked upstream, but allowlist URLs here just to be safe.

export const InvocationPage = observer(() => {
  const { invId } = useParams();
  const store = useStore();

  if (!invId) {
    throw new Error('invariant violated: invId must be set');
  }

  useEffect(() => {
    store.invocationPage.setInvocationId(invId);
  }, [invId, store]);

  const inv = store.invocationPage.invocation.invocation;
  const project = store.invocationPage.invocation.project;
  useProject(project || '');
  const parsedInvId = parseInvId(invId);

  return (
    <InvLitEnvProvider>
      <PageMeta title={`inv: ${invId}`} />
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
          {parsedInvId.type === 'build' && (
            <>
              {' '}
              (
              <a
                href={getBuildURLPathFromBuildId(parsedInvId.buildId)}
                target="_blank"
                rel="noreferrer"
              >
                build page
              </a>
              )
            </>
          )}
          {parsedInvId.type === 'swarming-task' && (
            <a
              href={getSwarmingTaskURL(
                parsedInvId.swarmingHost,
                parsedInvId.taskId,
              )}
              target="_blank"
              rel="noreferrer"
            >
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
      <LinearProgress
        value={100}
        variant={inv ? 'determinate' : 'indeterminate'}
      />
      <AppRoutedTabs>
        <AppRoutedTab
          label="Test Results"
          value="test-results"
          to="test-results"
          icon={<CountIndicator />}
          iconPosition="end"
        />
        <AppRoutedTab
          label="Invocation Details"
          value="invocation-details"
          to="invocation-details"
        />
      </AppRoutedTabs>
    </InvLitEnvProvider>
  );
});

export function Component() {
  return (
    <ContentGroup group="invocation">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="invocation"
      >
        <InvocationPage />
      </RecoverableErrorBoundary>
    </ContentGroup>
  );
}
