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
import { LinearProgress } from '@mui/material';
import { observer } from 'mobx-react-lite';
import { useEffect } from 'react';
import {
  Outlet,
  useNavigate,
  useParams,
  useSearchParams,
} from 'react-router-dom';

import { Tab, Tabs } from '@/common/components/tabs';
import {
  BUILD_STATUS_CLASS_MAP,
  BUILD_STATUS_COLOR_THEME_MAP,
  BUILD_STATUS_DISPLAY_MAP,
} from '@/common/constants';
import {
  GA_ACTIONS,
  GA_CATEGORIES,
  trackEvent,
} from '@/common/libs/analytics_utils';
import { displayDuration, LONG_TIME_FORMAT } from '@/common/libs/time_utils';
import {
  getBuilderURLPath,
  getBuildURLPath,
  getLegacyBuildURLPath,
  getProjectURLPath,
} from '@/common/libs/url_utils';
import { unwrapOrElse } from '@/common/libs/utils';
import { BuildStatus } from '@/common/services/buildbucket';
import { useStore } from '@/common/store';
import { InvocationProvider } from '@/common/store/invocation_state';

import { CountIndicator } from '../test_results_tab/count_indicator';

import { BuildLitEnvProvider } from './build_lit_env_provider';
import { ChangeConfigDialog } from './change_config_dialog';

const STATUS_FAVICON_MAP = Object.freeze({
  [BuildStatus.Scheduled]: 'gray',
  [BuildStatus.Started]: 'yellow',
  [BuildStatus.Success]: 'green',
  [BuildStatus.Failure]: 'red',
  [BuildStatus.InfraFailure]: 'purple',
  [BuildStatus.Canceled]: 'teal',
});

export const BuildPageShortLink = observer(() => {
  const { buildId, ['*']: pathSuffix } = useParams();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const store = useStore();

  if (!buildId) {
    throw new Error('invariant violated: buildId should be set');
  }

  useEffect(() => {
    store.buildPage.setParams(undefined, `b${buildId}`);
  }, [store, buildId]);

  const buildLoaded = Boolean(store.buildPage.build?.data);

  useEffect(() => {
    // Redirect to the long link after the build is fetched.
    if (!buildLoaded) {
      return;
    }
    const build = store.buildPage.build!;
    if (build.data.number !== undefined) {
      store.buildPage.setBuildId(
        build.data.builder,
        build.data.number,
        build.data.id
      );
    }
    const buildUrl = getBuildURLPath(build.data.builder, build.buildNumOrId);
    const searchString = searchParams.toString();
    const newUrl = `${buildUrl}${pathSuffix ? `/${pathSuffix}` : ''}${
      searchString ? `?${searchParams}` : ''
    }`;

    // TODO(weiweilin): sinon is not able to mock useNavigate.
    // Add a unit test once we setup jest.
    navigate(newUrl, { replace: true });
  }, [buildLoaded]);

  // Page will be redirected once the build is loaded.
  // Don't need to render anything.
  return <></>;
});

const delimiter = css({
  borderLeft: '1px solid var(--divider-color)',
  width: '1px',
  marginLeft: '10px',
  marginRight: '10px',
});

export const BuildPage = observer(() => {
  const { project, bucket, builder, buildNumOrId } = useParams();
  const store = useStore();

  if (!project || !bucket || !builder || !buildNumOrId) {
    throw new Error(
      'invariant violated: project, bucket, builder, buildNumOrId should be set'
    );
  }

  useEffect(() => {
    if (window.location.href.includes('javascript:')) {
      return;
    }
    trackEvent(
      GA_CATEGORIES.NEW_BUILD_PAGE,
      GA_ACTIONS.PAGE_VISITED,
      window.location.href
    );
    trackEvent(
      GA_CATEGORIES.PROJECT_BUILD_PAGE,
      GA_ACTIONS.VISITED_NEW,
      project
    );
  }, [project, bucket, builder, buildNumOrId]);

  useEffect(() => {
    store.registerSettingsDialog();
    return () => store.unregisterSettingsDialog();
  }, [store]);

  useEffect(() => {
    store.buildPage.setParams({ project, bucket, builder }, buildNumOrId);
  }, [store, project, bucket, builder, buildNumOrId]);

  const customBugLink = unwrapOrElse(
    () => store.buildPage.customBugLink,
    (err) => {
      console.error('failed to get the custom bug link', err);
      // Failing to get the bug link is Ok. Some users (e.g. CrOS partners)
      // may have access to the build but not the project configuration.
      return null;
    }
  );

  const build = store.buildPage.build;
  const buildURLPath = getBuildURLPath(
    { project, bucket, builder },
    buildNumOrId
  );

  const status = build?.data?.status;
  const statusDisplay = status ? BUILD_STATUS_DISPLAY_MAP[status] : 'loading';
  const documentTitle = `${statusDisplay} - ${builder} ${buildNumOrId}`;
  useEffect(() => {
    document.title = documentTitle;
  }, [documentTitle]);

  const faviconUrl = build
    ? `/static/common/favicon/${STATUS_FAVICON_MAP[build.data.status]}-32.png`
    : '/static/common/favicon/milo-32.png';
  useEffect(() => {
    document.getElementById('favicon')?.setAttribute('href', faviconUrl);
  }, [faviconUrl]);

  return (
    <InvocationProvider value={store.buildPage.invocation}>
      <BuildLitEnvProvider>
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
            <a href={getProjectURLPath(project)}>{project}</a>
            <span>&nbsp;/&nbsp;</span>
            <span>{bucket}</span>
            <span>&nbsp;/&nbsp;</span>
            <a href={getBuilderURLPath({ project, bucket, builder })}>
              {builder}
            </a>
            <span>&nbsp;/&nbsp;</span>
            <span>{buildNumOrId}</span>
          </div>
          {customBugLink && (
            <>
              <div css={delimiter}></div>
              <a href={customBugLink} target="_blank" rel="noreferrer">
                File a bug
              </a>
            </>
          )}
          {store.redirectSw && (
            <>
              <div css={delimiter}></div>
              <a
                onClick={(e) => {
                  const switchVerTemporarily =
                    e.metaKey || e.shiftKey || e.ctrlKey || e.altKey;
                  trackEvent(
                    GA_CATEGORIES.LEGACY_BUILD_PAGE,
                    switchVerTemporarily
                      ? GA_ACTIONS.SWITCH_VERSION_TEMP
                      : GA_ACTIONS.SWITCH_VERSION,
                    window.location.href
                  );

                  if (switchVerTemporarily) {
                    return;
                  }

                  const expires = new Date(
                    Date.now() + 365 * 24 * 60 * 60 * 1000
                  ).toUTCString();
                  document.cookie = `showNewBuildPage=false; expires=${expires}; path=/`;
                  store.redirectSw?.unregister();
                }}
                href={getLegacyBuildURLPath(
                  { project, bucket, builder },
                  buildNumOrId
                )}
              >
                Switch to the legacy build page
              </a>
            </>
          )}
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
                    case BuildStatus.Scheduled:
                      return `since ${build.createTime.toFormat(
                        LONG_TIME_FORMAT
                      )}`;
                    case BuildStatus.Started:
                      return `since ${build.startTime!.toFormat(
                        LONG_TIME_FORMAT
                      )}`;
                    case BuildStatus.Canceled:
                      return `after ${displayDuration(
                        build.endTime!.diff(build.createTime)
                      )} by ${build.data.canceledBy || 'unknown'}`;
                    case BuildStatus.Failure:
                    case BuildStatus.InfraFailure:
                    case BuildStatus.Success:
                      return `after ${displayDuration(
                        build.endTime!.diff(build.startTime || build.createTime)
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
            build ? BUILD_STATUS_COLOR_THEME_MAP[build.data.status] : 'primary'
          }
        />
        <Tabs value={store.selectedTabId || false}>
          <Tab
            label="Overview"
            value="overview"
            to={buildURLPath + '/overview'}
          />
          {/* If the tab is visited directly via URL before we know if it could
        exists, display the tab heading so <Tabs /> won't throw no matching tab
        error */}
          {(store.selectedTabId === 'test-results' ||
            (store.buildPage.hasInvocation &&
              store.buildPage.canReadTestVerdicts)) && (
            <Tab
              label="Test Results"
              value="test-results"
              to={buildURLPath + '/test-results'}
              icon={<CountIndicator />}
              iconPosition="end"
            />
          )}
          {(store.selectedTabId === 'steps' ||
            store.buildPage.canReadFullBuild) && (
            <Tab
              label="Steps & Logs"
              value="steps"
              to={buildURLPath + '/steps'}
            />
          )}
          {(store.selectedTabId === 'related-builds' ||
            store.buildPage.canReadFullBuild) && (
            <Tab
              label="Related Builds"
              value="related-builds"
              to={buildURLPath + '/related-builds'}
            />
          )}
          {(store.selectedTabId === 'timeline' ||
            store.buildPage.canReadFullBuild) && (
            <Tab
              label="Timeline"
              value="timeline"
              to={buildURLPath + '/timeline'}
            />
          )}
          {(store.selectedTabId === 'blamelist' ||
            store.buildPage.canReadFullBuild) && (
            <Tab
              label="Blamelist"
              value="blamelist"
              to={buildURLPath + '/blamelist'}
            />
          )}
        </Tabs>
        <Outlet />
      </BuildLitEnvProvider>
    </InvocationProvider>
  );
});
