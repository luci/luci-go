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

import '@material/mwc-button';
import '@material/mwc-dialog';
import '@material/mwc-icon';
import { BeforeEnterObserver, PreventAndRedirectCommands, Router, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { autorun, computed, makeObservable, observable, reaction, when } from 'mobx';
import { applySnapshot, destroy, getSnapshot } from 'mobx-state-tree';

import '../../components/status_bar';
import '../../components/tab_bar';
import '../test_results_tab/count_indicator';
import { OPTIONAL_RESOURCE } from '../../common_tags';
import { MiloBaseElement } from '../../components/milo_base';
import { TabDef } from '../../components/tab_bar';
import { BuildState, GetBuildError, provideBuildState } from '../../context/build_state';
import { InvocationState, provideInvocationState, QueryInvocationError } from '../../context/invocation_state';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import { getLegacyURLPathForBuild, getURLPathForBuilder, getURLPathForProject } from '../../libs/build_utils';
import {
  BUILD_STATUS_CLASS_MAP,
  BUILD_STATUS_COLOR_MAP,
  BUILD_STATUS_DISPLAY_MAP,
  POTENTIALLY_EXPIRED,
} from '../../libs/constants';
import { consumer, provider } from '../../libs/context';
import {
  errorHandler,
  forwardWithoutMsg,
  renderErrorInPre,
  reportError,
  reportRenderError,
} from '../../libs/error_handler';
import { attachTags, hasTags } from '../../libs/tag';
import { displayDuration, LONG_TIME_FORMAT } from '../../libs/time_utils';
import { LoadTestVariantsError } from '../../models/test_loader';
import { NOT_FOUND_URL, router } from '../../routes';
import { BuilderID, BuildStatus, TEST_PRESENTATION_KEY } from '../../services/buildbucket';
import { consumeStore, StoreInstance } from '../../store';
import { UserConfig } from '../../store/user_config';
import colorClasses from '../../styles/color_classes.css';
import commonStyle from '../../styles/common_style.css';

const STATUS_FAVICON_MAP = Object.freeze({
  [BuildStatus.Scheduled]: 'gray',
  [BuildStatus.Started]: 'yellow',
  [BuildStatus.Success]: 'green',
  [BuildStatus.Failure]: 'red',
  [BuildStatus.InfraFailure]: 'purple',
  [BuildStatus.Canceled]: 'teal',
});

// An array of [buildTabName, buildTabLabel] tuples.
// Use an array of tuples instead of an Object to ensure order.
const TAB_NAME_LABEL_TUPLES = Object.freeze([
  Object.freeze(['build-overview', 'Overview']),
  Object.freeze(['build-test-results', 'Test Results']),
  Object.freeze(['build-steps', 'Steps & Logs']),
  Object.freeze(['build-related-builds', 'Related Builds']),
  Object.freeze(['build-timeline', 'Timeline']),
  Object.freeze(['build-blamelist', 'Blamelist']),
]);

function retryWithoutComputedInvId(err: ErrorEvent, ele: BuildPageElement) {
  let recovered = false;
  if (err.error instanceof LoadTestVariantsError) {
    // Ignore request using the old invocation ID.
    if (!err.error.req.invocations.includes(`invocations/${ele.buildState.invocationId}`)) {
      recovered = true;
    }

    // Old builds don't support computed invocation ID.
    // Disable it and try again.
    if (ele.buildState.useComputedInvId && !err.error.req.pageToken) {
      ele.buildState.useComputedInvId = false;
      recovered = true;
    }
  } else if (err.error instanceof QueryInvocationError) {
    // Ignore request using the old invocation ID.
    if (err.error.invId !== ele.buildState.invocationId) {
      recovered = true;
    }

    // Old builds don't support computed invocation ID.
    // Disable it and try again.
    if (ele.buildState.useComputedInvId) {
      ele.buildState.useComputedInvId = false;
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
export class BuildPageElement extends MiloBaseElement implements BeforeEnterObserver {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref
  @provideBuildState({ global: true })
  buildState!: BuildState;

  @observable.ref
  @provideInvocationState({ global: true })
  invocationState!: InvocationState;

  @observable private uncommittedConfigs = UserConfig.create({});

  // The page is visited via a short link.
  // The page will be redirected to the long link after the build is fetched.
  private isShortLink = false;
  private urlSuffix = '';

  // builderParam is only set when the page visited via a full link.
  @observable.ref private builderIdParam?: BuilderID;
  @observable.ref private buildNumOrIdParam = '';

  @computed private get buildNumOrId() {
    return this.buildState.build?.buildNumOrId || this.buildNumOrIdParam;
  }

  @computed private get legacyUrl() {
    return getLegacyURLPathForBuild(this.builderIdParam!, this.buildNumOrId);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const buildId = location.params['build_id'];
    const path = location.params['path'];
    if (typeof buildId === 'string' && path instanceof Array) {
      this.isShortLink = true;
      this.buildNumOrIdParam = ('b' + buildId) as string;
      this.urlSuffix = '/' + path.join('/') + location.search + location.hash;
      return;
    }

    this.isShortLink = false;
    const project = location.params['project'];
    const bucket = location.params['bucket'];
    const builder = location.params['builder'];
    const buildNumOrId = location.params['build_num_or_id'];
    if ([project, bucket, builder, buildNumOrId].some((param) => typeof param !== 'string')) {
      return cmd.redirect(NOT_FOUND_URL);
    }

    this.builderIdParam = {
      project: project as string,
      bucket: bucket as string,
      builder: builder as string,
    };
    this.buildNumOrIdParam = buildNumOrId as string;
    return;
  }

  @computed private get faviconUrl() {
    if (this.buildState.build) {
      return `/static/common/favicon/${STATUS_FAVICON_MAP[this.buildState.build.status]}-32.png`;
    }
    return '/static/common/favicon/milo-32.png';
  }

  @computed private get documentTitle() {
    const status = this.buildState.build?.status;
    const statusDisplay = status ? BUILD_STATUS_DISPLAY_MAP[status] : 'loading';
    return `${statusDisplay} - ${this.builderIdParam?.builder || ''} ${this.buildNumOrId}`;
  }

  connectedCallback() {
    super.connectedCallback();
    if (!this.isShortLink && !window.location.href.includes('javascript:')) {
      trackEvent(GA_CATEGORIES.NEW_BUILD_PAGE, GA_ACTIONS.PAGE_VISITED, window.location.href);
      trackEvent(GA_CATEGORIES.PROJECT_BUILD_PAGE, GA_ACTIONS.VISITED_NEW, this.builderIdParam!.project);
    }

    this.store.registerSettingsDialog();

    this.addDisposer(
      reaction(
        () => [this.store],
        () => {
          this.buildState?.dispose();
          this.buildState = new BuildState(this.store);
          this.buildState.builderIdParam = this.builderIdParam;
          this.buildState.buildNumOrIdParam = this.buildNumOrIdParam;

          // Emulate @property() update.
          this.updated(new Map([['buildState', this.buildState]]));
        },
        { fireImmediately: true }
      )
    );
    this.addDisposer(() => this.buildState.dispose());

    this.addDisposer(
      reaction(
        () => this.store,
        (store) => {
          this.invocationState?.dispose();
          this.invocationState = new InvocationState(store);

          // Emulate @property() update.
          this.updated(new Map([['invocationState', this.invocationState]]));
        },
        { fireImmediately: true }
      )
    );
    this.addDisposer(() => this.invocationState.dispose());

    this.addDisposer(
      autorun(() => {
        this.invocationState.invocationId = this.buildState.invocationId;
        this.invocationState.isComputedInvId = this.buildState.useComputedInvId;
        this.invocationState.presentationConfig =
          this.buildState.build?.output?.properties?.[TEST_PRESENTATION_KEY] ||
          this.buildState.build?.input?.properties?.[TEST_PRESENTATION_KEY] ||
          {};
        this.invocationState.warning = this.buildState.build?.buildOrStepInfraFailed
          ? 'Test results displayed here are likely incomplete because some steps have infra failed.'
          : '';
      })
    );

    if (this.isShortLink) {
      // Redirect to the long link after the build is fetched.
      this.addDisposer(
        when(
          reportError(this, () => this.buildState.build !== null),
          () => {
            const build = this.buildState.build!;
            if (build.number !== undefined) {
              this.store.setBuildId(build.builder, build.number, build.id);
            }
            const buildUrl = router.urlForName('build', {
              project: build.builder.project,
              bucket: build.builder.bucket,
              builder: build.builder.builder,
              build_num_or_id: build.buildNumOrId,
            });

            const newUrl = buildUrl + this.urlSuffix;
            // Prevent the router from pushing the history state.
            window.history.replaceState(null, '', newUrl);
            Router.go(newUrl);
          }
        )
      );

      // Skip rendering-related reactions.
      return;
    }

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

    // Sync uncommitted configs with committed configs whenever user opens the
    // dialog.
    this.addDisposer(
      reaction(
        () => this.store.showSettingsDialog,
        (showSettingsDialog) => {
          if (showSettingsDialog) {
            applySnapshot(this.uncommittedConfigs, getSnapshot(this.store.userConfig));
          }
        },
        { fireImmediately: true }
      )
    );
  }

  disconnectedCallback() {
    this.store.unregisterSettingsDialog();
    destroy(this.uncommittedConfigs);
    super.disconnectedCallback();
  }

  @computed get tabDefs(): TabDef[] {
    const params = {
      project: this.builderIdParam!.project,
      bucket: this.builderIdParam!.bucket,
      builder: this.builderIdParam!.builder,
      build_num_or_id: this.buildNumOrIdParam,
    };
    return [
      {
        id: 'overview',
        label: 'Overview',
        href: router.urlForName('build-overview', params),
      },
      // TODO(crbug/1128097): display test-results tab unconditionally once
      // Foundation team is ready for ResultDB integration with other LUCI
      // projects.
      ...(!this.invocationState.hasInvocation
        ? []
        : [
            {
              id: 'test-results',
              label: 'Test Results',
              href: router.urlForName('build-test-results', params),
              slotName: 'test-count-indicator',
            },
          ]),
      {
        id: 'steps',
        label: 'Steps & Logs',
        href: router.urlForName('build-steps', params),
      },
      {
        id: 'related-builds',
        label: 'Related Builds',
        href: router.urlForName('build-related-builds', params),
      },
      {
        id: 'timeline',
        label: 'Timeline',
        href: router.urlForName('build-timeline', params),
      },
      {
        id: 'blamelist',
        label: 'Blamelist',
        href: router.urlForName('build-blamelist', params),
      },
    ];
  }

  @computed private get statusBarColor() {
    const build = this.buildState.build;
    return build ? BUILD_STATUS_COLOR_MAP[build.status] : 'var(--active-color)';
  }

  private renderBuildStatus() {
    const build = this.buildState.build;
    if (!build) {
      return html``;
    }
    return html`
      <i class="status ${BUILD_STATUS_CLASS_MAP[build.status]}">
        ${BUILD_STATUS_DISPLAY_MAP[build.status] || 'unknown status'}
      </i>
      ${(() => {
        switch (build.status) {
          case BuildStatus.Scheduled:
            return `since ${build.createTime.toFormat(LONG_TIME_FORMAT)}`;
          case BuildStatus.Started:
            return `since ${build.startTime!.toFormat(LONG_TIME_FORMAT)}`;
          case BuildStatus.Canceled:
            return `after ${displayDuration(build.endTime!.diff(build.createTime))} by ${
              build.canceledBy || 'unknown'
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
    if (this.isShortLink) {
      return html``;
    }

    return html`
      <mwc-dialog
        id="settings-dialog"
        heading="Settings"
        ?open=${this.store.showSettingsDialog}
        @closed=${(event: CustomEvent<{ action: string }>) => {
          if (event.detail.action === 'save') {
            applySnapshot(this.store.userConfig, getSnapshot(this.uncommittedConfigs));
          }
          // Reset uncommitted configs.
          destroy(this.uncommittedConfigs);
          this.uncommittedConfigs = UserConfig.create({});
          this.store.setShowSettingsDialog(false);
        }}
      >
        <table>
          <tr>
            <td>Default tab:</td>
            <td>
              <select
                id="default-tab-selector"
                @change=${(e: InputEvent) =>
                  this.uncommittedConfigs.build.setDefaultTab((e.target as HTMLOptionElement).value)}
              >
                ${TAB_NAME_LABEL_TUPLES.map(
                  ([tabName, label]) => html`
                    <option value=${tabName} ?selected=${tabName === this.uncommittedConfigs.build.defaultTabName}>
                      ${label}
                    </option>
                  `
                )}
              </select>
            </td>
          </tr>
          <mwc-button slot="primaryAction" dialogAction="save" dense unelevated>Save</mwc-button>
          <mwc-button slot="secondaryAction" dialogAction="dismiss">Cancel</mwc-button>
        </table>
      </mwc-dialog>
      <div id="build-summary">
        <div id="build-id">
          <span id="build-id-label">Build </span>
          <a href=${getURLPathForProject(this.builderIdParam!.project)}>${this.builderIdParam!.project}</a>
          <span>&nbsp;/&nbsp;</span>
          <span>${this.builderIdParam!.bucket}</span>
          <span>&nbsp;/&nbsp;</span>
          <a href=${getURLPathForBuilder(this.builderIdParam!)}>${this.builderIdParam!.builder}</a>
          <span>&nbsp;/&nbsp;</span>
          <span>${this.buildNumOrId}</span>
        </div>
        ${this.buildState.customBugLink === null
          ? html``
          : html`
              <div class="delimiter"></div>
              <a href=${this.buildState.customBugLink} target="_blank">File a bug</a>
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
        .loading=${!this.buildState.build}
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

      #settings-dialog {
        --mdc-dialog-min-width: 600px;
      }
      #default-tab-selector {
        display: inline-block;
        margin-left: 10px;
        padding: 0.375rem 0.75rem;
        font-size: 1rem;
        line-height: 1.5;
        background-clip: padding-box;
        border: 1px solid var(--divider-color);
        border-radius: 0.25rem;
        transition: border-color 0.15s ease-in-out, box-shadow 0.15s ease-in-out;
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
