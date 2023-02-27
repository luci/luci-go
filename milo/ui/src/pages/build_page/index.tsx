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

import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable, reaction } from 'mobx';
import { observer } from 'mobx-react-lite';
import { ReactNode, useEffect } from 'react';
import { Outlet, useNavigate, useParams } from 'react-router-dom';

import '../../components/status_bar';
import '../../components/tab_bar';
import '../test_results_tab/count_indicator';
import './change_config_dialog';
import { OPTIONAL_RESOURCE } from '../../common_tags';
import { MiloBaseElement } from '../../components/milo_base';
import { TabDef } from '../../components/tab_bar';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import {
  BUILD_STATUS_CLASS_MAP,
  BUILD_STATUS_COLOR_MAP,
  BUILD_STATUS_DISPLAY_MAP,
  POTENTIALLY_EXPIRED,
} from '../../libs/constants';
import { consumer, provider } from '../../libs/context';
import { errorHandler, forwardWithoutMsg, renderErrorInPre, reportRenderError } from '../../libs/error_handler';
import { attachTags, hasTags } from '../../libs/tag';
import { displayDuration, LONG_TIME_FORMAT } from '../../libs/time_utils';
import { getBuilderURLPath, getBuildURLPath, getLegacyBuildURLPath, getProjectURLPath } from '../../libs/url_utils';
import { unwrapOrElse } from '../../libs/utils';
import { LoadTestVariantsError } from '../../models/test_loader';
import { BuilderID, BuildStatus } from '../../services/buildbucket';
import { consumeStore, StoreInstance, useStore } from '../../store';
import { GetBuildError } from '../../store/build_page';
import { provideInvocationState, QueryInvocationError } from '../../store/invocation_state';
import colorClasses from '../../styles/color_classes.css';
import commonStyle from '../../styles/common_style.css';
import { provideProject } from '../test_results_tab/test_variants_table/context';

const STATUS_FAVICON_MAP = Object.freeze({
  [BuildStatus.Scheduled]: 'gray',
  [BuildStatus.Started]: 'yellow',
  [BuildStatus.Success]: 'green',
  [BuildStatus.Failure]: 'red',
  [BuildStatus.InfraFailure]: 'purple',
  [BuildStatus.Canceled]: 'teal',
});

function retryWithoutComputedInvId(err: ErrorEvent, ele: BuildPageElement) {
  let recovered = false;
  if (err.error instanceof LoadTestVariantsError) {
    // Ignore request using the old invocation ID.
    if (!err.error.req.invocations.includes(`invocations/${ele.store.buildPage.invocationId}`)) {
      recovered = true;
    }

    // Old builds don't support computed invocation ID.
    // Disable it and try again.
    if (ele.store.buildPage.useComputedInvId && !err.error.req.pageToken) {
      ele.store.buildPage.setUseComputedInvId(false);
      recovered = true;
    }
  } else if (err.error instanceof QueryInvocationError) {
    // Ignore request using the old invocation ID.
    if (err.error.invId !== ele.store.buildPage.invocationId) {
      recovered = true;
    }

    // Old builds don't support computed invocation ID.
    // Disable it and try again.
    if (ele.store.buildPage.useComputedInvId) {
      ele.store.buildPage.setUseComputedInvId(false);
      recovered = true;
    }
  }

  if (recovered) {
    err.stopImmediatePropagation();
    err.preventDefault();
    return false;
  }

  if (!(err.error instanceof GetBuildError)) {
    attachTags(err.error, OPTIONAL_RESOURCE);
  }

  return forwardWithoutMsg(err, ele);
}

function renderError(err: ErrorEvent, ele: BuildPageElement) {
  if (err.error instanceof GetBuildError && hasTags(err.error, POTENTIALLY_EXPIRED)) {
    return html`
      <div id="build-not-found-error">
        Build Not Found: if you are trying to view an old build, it could have been wiped from the server already.
      </div>
      ${renderErrorInPre(err, ele)}
    `;
  }

  return renderErrorInPre(err, ele);
}

/**
 * Main build page.
 * Reads project, bucket, builder and build from URL params.
 * If any of the parameters are not provided, redirects to '/not-found'.
 * If build is not a number, shows an error.
 */
@customElement('milo-build-page')
@errorHandler(retryWithoutComputedInvId, renderError)
@provider
@consumer
export class BuildPageElement extends MiloBaseElement {
  static get properties() {
    return {
      builderIdParam: { type: Object, attribute: 'builder-id-param' },
      buildNumOrIdParam: { type: String, attribute: 'build-num-or-id-param' },
    };
  }

  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @computed private get build() {
    return this.store.buildPage.build;
  }

  @provideInvocationState({ global: true })
  @computed
  get invState() {
    return this.store.buildPage.invocation;
  }

  @provideProject({ global: true })
  @computed
  get project() {
    return this.store.buildPage.build?.data.builder.project;
  }

  @observable.ref private _builderIdParam!: BuilderID;
  @computed get builderIdParam() {
    return this._builderIdParam;
  }
  set builderIdParam(newVal: BuilderID) {
    this._builderIdParam = newVal;
  }

  @observable.ref private _buildNumOrIdParam = '';
  @computed get buildNumOrIdParam() {
    return this._buildNumOrIdParam;
  }
  set buildNumOrIdParam(newVal: string) {
    this._buildNumOrIdParam = newVal;
  }

  @computed private get buildNumOrId() {
    return this.store.buildPage.build?.buildNumOrId || this.buildNumOrIdParam;
  }

  @computed private get legacyUrl() {
    return getLegacyBuildURLPath(this.builderIdParam!, this.buildNumOrId);
  }

  @computed private get customBugLink() {
    return unwrapOrElse(
      () => this.store.buildPage.customBugLink,
      (err) => {
        console.error('failed to get the custom bug link', err);
        // Failing to get the bug link is Ok. Some users (e.g. CrOS partners)
        // may have access to the build but not the project configuration.
        return null;
      }
    );
  }

  constructor() {
    super();
    makeObservable(this);
  }

  @computed private get faviconUrl() {
    if (this.build?.data) {
      return `/static/common/favicon/${STATUS_FAVICON_MAP[this.build?.data.status]}-32.png`;
    }
    return '/static/common/favicon/milo-32.png';
  }

  @computed private get documentTitle() {
    const status = this.build?.data?.status;
    const statusDisplay = status ? BUILD_STATUS_DISPLAY_MAP[status] : 'loading';
    return `${statusDisplay} - ${this.builderIdParam?.builder || ''} ${this.buildNumOrId}`;
  }

  connectedCallback() {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.invState,
        (invState) => {
          // Emulate @property() update.
          this.updated(new Map([['invState', invState]]));
        },
        { fireImmediately: true }
      )
    );

    this.addDisposer(
      reaction(
        () => this.project,
        (project) => {
          // Emulate @property() update.
          this.updated(new Map([['project', project]]));
        },
        { fireImmediately: true }
      )
    );

    this.addDisposer(
      reaction(
        () => this.faviconUrl,
        (faviconUrl) => document.getElementById('favicon')?.setAttribute('href', faviconUrl),
        { fireImmediately: true }
      )
    );

    this.addDisposer(
      reaction(
        () => this.documentTitle,
        (title) => (document.title = title),
        { fireImmediately: true }
      )
    );
  }

  @computed get tabDefs(): TabDef[] {
    const buildURLPath = getBuildURLPath(this.builderIdParam, this.buildNumOrIdParam);
    return [
      {
        id: 'overview',
        label: 'Overview',
        href: buildURLPath + '/overview',
      },
      // TODO(crbug/1128097): display test-results tab unconditionally once
      // Foundation team is ready for ResultDB integration with other LUCI
      // projects.
      ...(!this.store.buildPage.hasInvocation || !this.store.buildPage.canReadTestVerdicts
        ? []
        : [
            {
              id: 'test-results',
              label: 'Test Results',
              href: buildURLPath + '/test-results',
              slotName: 'test-count-indicator',
            },
          ]),
      ...(!this.store.buildPage.canReadFullBuild
        ? []
        : [
            {
              id: 'steps',
              label: 'Steps & Logs',
              href: buildURLPath + '/steps',
            },
            {
              id: 'related-builds',
              label: 'Related Builds',
              href: buildURLPath + '/related-builds',
            },
            {
              id: 'timeline',
              label: 'Timeline',
              href: buildURLPath + '/timeline',
            },
            {
              id: 'blamelist',
              label: 'Blamelist',
              href: buildURLPath + '/blamelist',
            },
          ]),
    ];
  }

  @computed private get statusBarColor() {
    const build = this.store.buildPage.build?.data;
    return build ? BUILD_STATUS_COLOR_MAP[build.status] : 'var(--active-color)';
  }

  private renderBuildStatus() {
    const build = this.store.buildPage.build;
    if (!build) {
      return html``;
    }
    return html`
      <i class="status ${BUILD_STATUS_CLASS_MAP[build.data.status]}">
        ${BUILD_STATUS_DISPLAY_MAP[build.data.status] || 'unknown status'}
      </i>
      ${(() => {
        switch (build.data.status) {
          case BuildStatus.Scheduled:
            return `since ${build.createTime.toFormat(LONG_TIME_FORMAT)}`;
          case BuildStatus.Started:
            return `since ${build.startTime!.toFormat(LONG_TIME_FORMAT)}`;
          case BuildStatus.Canceled:
            return `after ${displayDuration(build.endTime!.diff(build.createTime))} by ${
              build.data.canceledBy || 'unknown'
            }`;
          case BuildStatus.Failure:
          case BuildStatus.InfraFailure:
          case BuildStatus.Success:
            return `after ${displayDuration(build.endTime!.diff(build.startTime || build.createTime))}`;
          default:
            return '';
        }
      })()}
    `;
  }

  protected render = reportRenderError(this, () => {
    return html`
      <milo-bp-change-config-dialog
        .open=${this.store.showSettingsDialog}
        @close=${() => this.store.setShowSettingsDialog(false)}
      ></milo-bp-change-config-dialog>
      <div id="build-summary">
        <div id="build-id">
          <span id="build-id-label">Build </span>
          <a href=${getProjectURLPath(this.builderIdParam.project)}>${this.builderIdParam.project}</a>
          <span>&nbsp;/&nbsp;</span>
          <span>${this.builderIdParam.bucket}</span>
          <span>&nbsp;/&nbsp;</span>
          <a href=${getBuilderURLPath(this.builderIdParam)}>${this.builderIdParam.builder}</a>
          <span>&nbsp;/&nbsp;</span>
          <span>${this.buildNumOrId}</span>
        </div>
        ${this.customBugLink === null
          ? html``
          : html`
              <div class="delimiter"></div>
              <a href=${this.customBugLink} target="_blank">File a bug</a>
            `}
        ${this.store.redirectSw === null
          ? html``
          : html`
              <div class="delimiter"></div>
              <a
                @click=${(e: MouseEvent) => {
                  const switchVerTemporarily = e.metaKey || e.shiftKey || e.ctrlKey || e.altKey;
                  trackEvent(
                    GA_CATEGORIES.LEGACY_BUILD_PAGE,
                    switchVerTemporarily ? GA_ACTIONS.SWITCH_VERSION_TEMP : GA_ACTIONS.SWITCH_VERSION,
                    window.location.href
                  );

                  if (switchVerTemporarily) {
                    return;
                  }

                  const expires = new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toUTCString();
                  document.cookie = `showNewBuildPage=false; expires=${expires}; path=/`;
                  this.store.redirectSw?.unregister();
                }}
                href=${this.legacyUrl}
              >
                Switch to the legacy build page
              </a>
            `}
        <div id="build-status">${this.renderBuildStatus()}</div>
      </div>
      <milo-status-bar
        .components=${[{ color: this.statusBarColor, weight: 1 }]}
        .loading=${!this.store.buildPage.build}
      ></milo-status-bar>
      <milo-tab-bar .tabs=${this.tabDefs} .selectedTabId=${this.store.selectedTabId}>
        <milo-trt-count-indicator slot="test-count-indicator"></milo-trt-count-indicator>
      </milo-tab-bar>
      <slot></slot>
    `;
  });

  static styles = [
    commonStyle,
    colorClasses,
    css`
      #build-summary {
        background-color: var(--block-background-color);
        padding: 6px 16px;
        font-family: 'Google Sans', 'Helvetica Neue', sans-serif;
        font-size: 14px;
        display: flex;
      }

      #build-id {
        flex: 0 auto;
        font-size: 0px;
      }
      #build-id > * {
        font-size: 14px;
      }

      #build-id-label {
        color: var(--light-text-color);
      }

      #build-status {
        margin-left: auto;
        flex: 0 auto;
      }

      .delimiter {
        border-left: 1px solid var(--divider-color);
        width: 1px;
        margin-left: 10px;
        margin-right: 10px;
      }

      #status {
        font-weight: 500;
      }

      milo-tab-bar {
        margin: 0 10px;
        padding-top: 10px;
      }

      milo-trt-count-indicator {
        margin-right: -13px;
      }

      #build-not-found-error {
        background-color: var(--warning-color);
        font-weight: 500;
        padding: 5px;
        margin: 8px 16px;
      }
    `,
  ];
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-build-page': {
        'builder-id-param': string;
        'build-num-or-id-param': string;
        children: ReactNode;
      };
    }
  }
}

export const BuildPageShortLink = observer(() => {
  const { buildId, ['*']: pathSuffix } = useParams();
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
      store.buildPage.setBuildId(build.data.builder, build.data.number, build.data.id);
    }
    const buildUrl = getBuildURLPath(build.data.builder, build.buildNumOrId);
    const newUrl = '/ui' + buildUrl + (pathSuffix ? `/${pathSuffix}` : '');

    navigate(newUrl, { replace: true });
  }, [buildLoaded]);

  // Page will be redirected once the build is loaded.
  // Don't need to render anything.
  return <></>;
});

export const BuildPage = observer(() => {
  const { project, bucket, builder, buildNumOrId } = useParams();
  const store = useStore();

  if (!project || !bucket || !builder || !buildNumOrId) {
    throw new Error('invariant violated: project, bucket, builder, buildNumOrId should be set');
  }

  useEffect(() => {
    if (window.location.href.includes('javascript:')) {
      return;
    }
    trackEvent(GA_CATEGORIES.NEW_BUILD_PAGE, GA_ACTIONS.PAGE_VISITED, window.location.href);
    trackEvent(GA_CATEGORIES.PROJECT_BUILD_PAGE, GA_ACTIONS.VISITED_NEW, project);
  }, [project, bucket, builder, buildNumOrId]);

  useEffect(() => {
    store.registerSettingsDialog();
    return () => store.unregisterSettingsDialog();
  }, [store]);

  useEffect(() => {
    store.buildPage.setParams({ project, bucket, builder }, buildNumOrId);
  }, [store, project, bucket, builder, buildNumOrId]);

  return (
    <milo-build-page
      builder-id-param={JSON.stringify({ project, bucket, builder })}
      build-num-or-id-param={buildNumOrId}
    >
      <Outlet />
    </milo-build-page>
  );
});
