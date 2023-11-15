// Copyright 2020 The LUCI Authors.
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

import { css } from '@emotion/react';
import { LinearProgress, Link } from '@mui/material';
import { observer } from 'mobx-react-lite';
import { useEffect } from 'react';
import { Link as RouterLink, useParams } from 'react-router-dom';

import grayFavicon from '@/common/assets/favicons/gray-32.png';
import greenFavicon from '@/common/assets/favicons/green-32.png';
import miloFavicon from '@/common/assets/favicons/milo-32.png';
import purpleFavicon from '@/common/assets/favicons/purple-32.png';
import redFavicon from '@/common/assets/favicons/red-32.png';
import tealFavicon from '@/common/assets/favicons/teal-32.png';
import yellowFavicon from '@/common/assets/favicons/yellow-32.png';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta/page_meta';
import { AppRoutedTab, AppRoutedTabs } from '@/common/components/routed_tabs';
import {
  BUILD_STATUS_CLASS_MAP,
  BUILD_STATUS_COLOR_THEME_MAP,
  BUILD_STATUS_DISPLAY_MAP,
  UiPage,
} from '@/common/constants';
import { BuildbucketStatus } from '@/common/services/buildbucket';
import { useStore } from '@/common/store';
import { InvocationProvider } from '@/common/store/invocation_state';
import { displayDuration, LONG_TIME_FORMAT } from '@/common/tools/time_utils';
import {
  getOldBuilderURLPath,
  getLegacyBuildURLPath,
  getProjectURLPath,
} from '@/common/tools/url_utils';

import { CountIndicator } from '../test_results_tab/count_indicator';

import { BuildLitEnvProvider } from './build_lit_env_provider';
import { ChangeConfigDialog } from './change_config_dialog';
import { BuildContextProvider } from './context';
import { CustomBugLink } from './custom_bug_link';

const STATUS_FAVICON_MAP = Object.freeze({
  [BuildbucketStatus.Scheduled]: grayFavicon,
  [BuildbucketStatus.Started]: yellowFavicon,
  [BuildbucketStatus.Success]: greenFavicon,
  [BuildbucketStatus.Failure]: redFavicon,
  [BuildbucketStatus.InfraFailure]: purpleFavicon,
  [BuildbucketStatus.Canceled]: tealFavicon,
});

const delimiter = css({
  borderLeft: '1px solid var(--divider-color)',
  width: '1px',
  marginLeft: '10px',
  marginRight: '10px',
  '& + &': {
    display: 'none',
  },
});

export const BuildPage = observer(() => {
  const { project, bucket, builder, buildNumOrId } = useParams();
  const store = useStore();

  if (!project || !bucket || !builder || !buildNumOrId) {
    throw new Error(
      'invariant violated: project, bucket, builder, buildNumOrId should be set',
    );
  }

  useEffect(() => {
    store.registerSettingsDialog();
    return () => store.unregisterSettingsDialog();
  }, [store]);

  useEffect(() => {
    store.buildPage.setParams({ project, bucket, builder }, buildNumOrId);
  }, [store, project, bucket, builder, buildNumOrId]);

  const build = store.buildPage.build;

  const status = build?.data?.status;
  const statusDisplay = status ? BUILD_STATUS_DISPLAY_MAP[status] : 'loading';
  const documentTitle = `${statusDisplay} - ${builder} ${buildNumOrId}`;

  const faviconUrl = build
    ? STATUS_FAVICON_MAP[build.data.status]
    : miloFavicon;
  useEffect(() => {
    document.getElementById('favicon')?.setAttribute('href', faviconUrl);
  }, [faviconUrl]);

  const handleSwitchVersion = (
    e: React.MouseEvent<HTMLAnchorElement, MouseEvent>,
  ) => {
    const switchVerTemporarily =
      e.metaKey || e.shiftKey || e.ctrlKey || e.altKey;

    if (switchVerTemporarily) {
      return;
    }

    const expires = new Date(
      Date.now() + 365 * 24 * 60 * 60 * 1000,
    ).toUTCString();
    document.cookie = `showNewBuildPage=false; expires=${expires}; path=/`;
    store.redirectSw?.unregister();
  };

  return (
    <BuildContextProvider build={store.buildPage.build?.data || null}>
      <InvocationProvider value={store.buildPage.invocation}>
        <BuildLitEnvProvider>
          <PageMeta
            project={project}
            selectedPage={UiPage.Builders}
            title={documentTitle}
          />
          <ChangeConfigDialog
            open={store.showSettingsDialog}
            onClose={() => store.setShowSettingsDialog(false)}
          />
          <div
            css={{
              backgroundColor: 'var(--block-background-color)',
              padding: '6px 16px',
              fontFamily: "'Google Sans', 'Helvetica Neue', sans-serif",
              fontSize: '14px',
              display: 'flex',
            }}
          >
            <div
              css={{
                flex: '0 auto',
                fontSize: '0px',
                '& > *': {
                  fontSize: '14px',
                },
              }}
            >
              <span css={{ color: 'var(--light-text-color)' }}>Build </span>
              <Link component={RouterLink} to={getProjectURLPath(project)}>
                {project}
              </Link>
              <span>&nbsp;/&nbsp;</span>
              <span>{bucket}</span>
              <span>&nbsp;/&nbsp;</span>
              <Link href={getOldBuilderURLPath({ project, bucket, builder })}>
                {builder}
              </Link>
              <span>&nbsp;/&nbsp;</span>
              <span>{buildNumOrId}</span>
            </div>
            <div css={delimiter}></div>
            <CustomBugLink project={project} build={build?.data} />
            <div css={delimiter}></div>
            <Link
              onClick={handleSwitchVersion}
              href={getLegacyBuildURLPath(
                { project, bucket, builder },
                buildNumOrId,
              )}
            >
              Switch to the legacy build page
            </Link>
            <div
              css={{
                marginLeft: 'auto',
                flex: '0 auto',
              }}
            >
              {build && (
                <>
                  <i
                    className={`status ${
                      BUILD_STATUS_CLASS_MAP[build.data.status]
                    }`}
                  >
                    {BUILD_STATUS_DISPLAY_MAP[build.data.status] ||
                      'unknown status'}{' '}
                  </i>
                  {(() => {
                    switch (build.data.status) {
                      case BuildbucketStatus.Scheduled:
                        return `since ${build.createTime.toFormat(
                          LONG_TIME_FORMAT,
                        )}`;
                      case BuildbucketStatus.Started:
                        return `since ${build.startTime!.toFormat(
                          LONG_TIME_FORMAT,
                        )}`;
                      case BuildbucketStatus.Canceled:
                        return `after ${displayDuration(
                          build.endTime!.diff(build.createTime),
                        )} by ${build.data.canceledBy || 'unknown'}`;
                      case BuildbucketStatus.Failure:
                      case BuildbucketStatus.InfraFailure:
                      case BuildbucketStatus.Success:
                        return `after ${displayDuration(
                          build.endTime!.diff(
                            build.startTime || build.createTime,
                          ),
                        )}`;
                      default:
                        return '';
                    }
                  })()}
                </>
              )}
            </div>
          </div>
          <LinearProgress
            value={100}
            variant={build ? 'determinate' : 'indeterminate'}
            color={
              build
                ? BUILD_STATUS_COLOR_THEME_MAP[build.data.status]
                : 'primary'
            }
          />
          <AppRoutedTabs>
            <AppRoutedTab label="Overview" value="overview" to="overview" />
            <AppRoutedTab
              label="Test Results"
              value="test-results"
              to="test-results"
              hideWhenInactive={
                !store.buildPage.hasInvocation ||
                !store.buildPage.canReadTestVerdicts
              }
              icon={<CountIndicator />}
              iconPosition="end"
            />
            <AppRoutedTab
              label="Steps & Logs"
              value="steps"
              to="steps"
              hideWhenInactive={!store.buildPage.canReadFullBuild}
            />
            <AppRoutedTab
              label="Related Builds"
              value="related-builds"
              to="related-builds"
              hideWhenInactive={!store.buildPage.canReadFullBuild}
            />
            <AppRoutedTab
              label="Timeline"
              value="timeline"
              to="timeline"
              hideWhenInactive={!store.buildPage.canReadFullBuild}
            />
            <AppRoutedTab
              label="Blamelist"
              value="blamelist"
              to="blamelist"
              hideWhenInactive={!store.buildPage.canReadFullBuild}
            />
          </AppRoutedTabs>
        </BuildLitEnvProvider>
      </InvocationProvider>
    </BuildContextProvider>
  );
});

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="build-long-link">
    <BuildPage />
  </RecoverableErrorBoundary>
);
