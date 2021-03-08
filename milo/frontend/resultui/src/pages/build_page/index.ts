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

import { MobxLitElement } from '@adobe/lit-mobx';
import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import '@material/mwc-button';
import '@material/mwc-dialog';
import '@material/mwc-icon';
import { BeforeEnterObserver, PreventAndRedirectCommands, Router, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import merge from 'lodash-es/merge';
import { autorun, computed, observable, reaction, when } from 'mobx';
import { FULFILLED, PENDING, REJECTED } from 'mobx-utils';

import '../../components/status_bar';
import '../../components/tab_bar';
import { TabDef } from '../../components/tab_bar';
import { AppState, consumeAppState } from '../../context/app_state';
import { BuildState, provideBuildState } from '../../context/build_state';
import { InvocationState, provideInvocationState, QueryInvocationError } from '../../context/invocation_state';
import { consumeConfigsStore, DEFAULT_USER_CONFIGS, UserConfigs, UserConfigsStore } from '../../context/user_configs';
import { getGitilesRepoURL, getLegacyURLForBuild, getURLForBuilder, getURLForProject } from '../../libs/build_utils';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_COLOR_MAP, BUILD_STATUS_DISPLAY_MAP } from '../../libs/constants';
import { displayDuration, LONG_TIME_FORMAT } from '../../libs/time_utils';
import { genFeedbackUrl } from '../../libs/utils';
import { LoadTestVariantsError } from '../../models/test_loader';
import { NOT_FOUND_URL, router } from '../../routes';
import { BuilderID, BuildStatus } from '../../services/buildbucket';

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

/**
 * Main build page.
 * Reads project, bucket, builder and build from URL params.
 * If any of the parameters are not provided, redirects to '/not-found'.
 * If build is not a number, shows an error.
 */
export class BuildPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  @observable.ref configsStore!: UserConfigsStore;
  @observable.ref buildState!: BuildState;
  @observable.ref invocationState!: InvocationState;

  // Set to true when testing.
  // Otherwise this.render() will throw due to missing initialization steps in
  // this.onBeforeEnter.
  @observable.ref prerender = false;

  @observable private readonly uncommittedConfigs: UserConfigs = merge({}, DEFAULT_USER_CONFIGS);
  @observable.ref private showFeedbackDialog = false;

  // The page is visited via a short link.
  // The page will be redirected to the long link after the build is fetched.
  private isShortLink = false;
  private urlSuffix = '';

  // builderParam is only set when the page visited via a full link.
  private builderParam?: BuilderID;
  private buildNumOrIdParam = '';

  private get legacyUrl() {
    return getLegacyURLForBuild(this.builderParam!, this.buildNumOrIdParam);
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const buildId = location.params['build_id'];
    const path = location.params['path'];
    if (typeof buildId === 'string' && path instanceof Array) {
      this.isShortLink = true;
      this.buildNumOrIdParam = 'b' + buildId as string;
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

    this.builderParam = {
      project: project as string,
      bucket: bucket as string,
      builder: builder as string,
    };
    this.buildNumOrIdParam = buildNumOrId as string;
    return;
  }

  private disposers: Array<() => void> = [];

  @computed private get faviconUrl() {
    if (this.buildState.build) {
      return `/static/common/favicon/${STATUS_FAVICON_MAP[this.buildState.build.status]}-32.png`;
    }
    return `/static/common/favicon/milo-32.png`;
  }

  @computed private get documentTitle() {
    const status = this.buildState.build?.status;
    const statusDisplay = status ? BUILD_STATUS_DISPLAY_MAP[status] : 'loading';
    return `${statusDisplay} - ${this.builderParam?.builder || ''} ${this.buildNumOrIdParam}`;
  }

  private errorHandler = (e: ErrorEvent) => {
    if (e.error instanceof LoadTestVariantsError) {
      // Ignore request using the old invocation ID.
      if (!e.error.req.invocations.includes(`invocations/${this.buildState.invocationId}`)) {
        e.stopPropagation();
        return;
      }

      // Old builds don't support computed invocation ID.
      // Disable it and try again.
      if (this.buildState.useComputedInvId && !e.error.req.pageToken) {
        this.buildState.useComputedInvId = false;
        e.stopPropagation();
        return;
      }
    }
  }

  connectedCallback() {
    super.connectedCallback();
    this.appState.hasSettingsDialog++;

    this.addEventListener('error', this.errorHandler);

    this.disposers.push(reaction(
      () => [this.appState],
      () => {
        this.buildState?.dispose();
        this.buildState = new BuildState(this.appState);
        this.buildState.builder = this.builderParam;
        this.buildState.buildNumOrId = this.buildNumOrIdParam;

        // Emulate @property() update.
        this.updated(new Map([['buildState', this.buildState]]));
      },
      {fireImmediately: true},
    ));
    this.disposers.push(() => this.buildState.dispose());

    this.disposers.push(autorun(
      () => {
        if (this.buildState.build$.state !== REJECTED) {
          return;
        }
        const err = this.buildState.build$.value as GrpcError;
        // If the build is not found and the user is not logged in, redirect
        // them to the login page.
        if (err.code === RpcCode.NOT_FOUND && this.appState.accessToken === '') {
          Router.go(`${router.urlForName('login')}?${new URLSearchParams([['redirect', window.location.href]])}`);
          return;
        }
        this.dispatchEvent(new ErrorEvent('error', {
          message: this.buildState.build$.value.toString(),
          composed: true,
          bubbles: true,
        }));
      },
    ));

    this.disposers.push(reaction(
      () => this.appState,
      (appState) => {
        this.invocationState?.dispose();
        this.invocationState = new InvocationState(appState);
        this.invocationState.invocationId = this.buildState.invocationId;

        // Emulate @property() update.
        this.updated(new Map([['invocationState', this.invocationState]]));
      },
      {fireImmediately: true},
    ));
    this.disposers.push(() => this.invocationState.dispose());

    this.disposers.push(reaction(
      () => this.buildState.invocationId,
      (invId) => this.invocationState.invocationId = invId,
      {fireImmediately: true},
    ));

    this.disposers.push(reaction(
      () => this.invocationState.invocation$.state,
      () => {
        if (this.invocationState.invocation$.state !== REJECTED) {
          return;
        }
        const err = this.invocationState.invocation$.value as QueryInvocationError;
        // Ignore request using the old invocation ID.
        if (err.invId !== this.buildState.invocationId) {
          return;
        }
        // Old builds don't support computed invocation ID.
        // Disable it and try again.
        if (this.buildState.useComputedInvId) {
          this.buildState.useComputedInvId = false;
          return;
        }
        this.dispatchEvent(new ErrorEvent('error', {
          message: this.invocationState.invocation$.value.toString(),
          composed: true,
          bubbles: true,
        }));
      },
    ));

    if (this.isShortLink) {
      // Redirect to the long link after the build is fetched.
      this.disposers.push(when(
        () => this.buildState.build$.state === FULFILLED,
        () => {
          const builder = this.buildState.build!.builder;
          const buildUrl = router.urlForName(
            'build',
            {
              project: builder.project,
              bucket: builder.bucket,
              builder: builder.builder,
              build_num_or_id: this.buildNumOrIdParam,
            },
          );
          Router.go(buildUrl + this.urlSuffix);
        },
      ));

      // Skip rendering-related reactions.
      return;
    }

    this.disposers.push(autorun(() => {
      const build = this.buildState.build;
      if (!build) {
        return;
      }

      // If the build has only succeeded steps, show all steps in the steps tab by
      // default (even if the user's preference is to hide succeeded steps).
      if (build.rootSteps.every((s) => s.status === BuildStatus.Success)) {
        this.configsStore.userConfigs.steps.showSucceededSteps = true;
      }

      // If the input gitiles commit is in the blamelist pins, select it.
      // Otherwise, select the first blamelist pin.
      const buildInputCommitRepo = build.input.gitilesCommit
        ? getGitilesRepoURL(build.input.gitilesCommit)
        : null;
      let selectedBlamelistPinIndex = build.blamelistPins
        .findIndex((pin) => getGitilesRepoURL(pin) === buildInputCommitRepo) || 0;
      if (selectedBlamelistPinIndex === -1) {
        selectedBlamelistPinIndex = 0;
      }
      this.appState.selectedBlamelistPinIndex = selectedBlamelistPinIndex;
    }));

    this.disposers.push((reaction(
      () => this.faviconUrl,
      (faviconUrl) => document.getElementById('favicon')?.setAttribute('href', faviconUrl),
      {fireImmediately: true},
    )));

    this.disposers.push((reaction(
      () => this.documentTitle,
      (title) => document.title = title,
      {fireImmediately: true},
    )));

    // Sync uncommitted configs with committed configs.
    this.disposers.push(reaction(
      () => merge({}, this.configsStore.userConfigs),
      (committedConfig) => merge(this.uncommittedConfigs, committedConfig),
      {fireImmediately: true},
    ));
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
    this.removeEventListener('error', this.errorHandler);
    this.appState.hasSettingsDialog--;
  }

  @computed get hasInvocation() {
    if (this.buildState.useComputedInvId) {
      // The invocation may not exist. Wait for the invocation query to confirm
      // its existence.
      return this.invocationState.invocation !== null;
    }
    return Boolean(this.invocationState.invocationId);
  }

  @computed get tabDefs(): TabDef[] {
    const params = {
      'project': this.builderParam!.project,
      'bucket': this.builderParam!.bucket,
      'builder': this.builderParam!.builder,
      'build_num_or_id': this.buildNumOrIdParam,
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
      ...!this.hasInvocation ? [] : [{
        id: 'test-results',
        label: 'Test Results',
        href: router.urlForName('build-test-results', params),
      }],
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
      ${(() => { switch (build.status) {
      case BuildStatus.Scheduled:
        return `since ${build.createTime.toFormat(LONG_TIME_FORMAT)}`;
      case BuildStatus.Started:
        return `since ${build.startTime!.toFormat(LONG_TIME_FORMAT)}`;
      case BuildStatus.Canceled:
        return `after ${displayDuration(build.endTime!.diff(build.createTime))} by ${build.canceledBy}`;
      case BuildStatus.Failure:
      case BuildStatus.InfraFailure:
      case BuildStatus.Success:
        return `after ${displayDuration(build.endTime!.diff(build.startTime || build.createTime))}`;
      default:
        return '';
      }})()}
    `;
  }

  protected render() {
    if (this.isShortLink || this.prerender) {
      return html``;
    }

    return html`
      <mwc-dialog
        id="settings-dialog"
        heading="Settings"
        ?open=${this.appState.showSettingsDialog}
        @closed=${(event: CustomEvent<{action: string}>) => {
          if (event.detail.action === 'save') {
            merge(this.configsStore.userConfigs, this.uncommittedConfigs);
            this.configsStore.save();
          }
          // Reset uncommitted configs.
          merge(this.uncommittedConfigs, this.configsStore.userConfigs);
          this.appState.showSettingsDialog = false;
        }}
      >
        <label for="default-tab-selector">Default tab:</label>
        <select
          id="default-tab-selector"
          @change=${(e: InputEvent) => this.uncommittedConfigs.defaultBuildPageTabName = (e.target as HTMLOptionElement).value}
        >
          ${TAB_NAME_LABEL_TUPLES.map(([tabName, label]) => html`
          <option
            value=${tabName}
            ?selected=${tabName === this.uncommittedConfigs.defaultBuildPageTabName}
          >${label}</option>
          `)}
        </select>
        <mwc-button slot="primaryAction" dialogAction="save" dense unelevated>Save</mwc-button>
        <mwc-button slot="secondaryAction" dialogAction="dismiss">Cancel</mwc-button>
      </mwc-dialog>
      <mwc-dialog
        id="feedback-dialog"
        heading="Tell Us What's Missing"
        ?open=${this.showFeedbackDialog}
        @closed=${() => {
          const noFeedbackEle = this.shadowRoot!.getElementById('no-feedback-prompt')! as HTMLInputElement;
          if (noFeedbackEle.checked) {
            this.configsStore.userConfigs.askForFeedback = false;
            this.configsStore.save();
          }
          this.showFeedbackDialog = false;
          window.open(this.legacyUrl, '_self');
        }}
      >
        <div>
          We'd love to make the new build page work better for everyone.<br>
          Please take a moment to give us feedback before switching back to the old build page.
        </div>
        <br>
        <input type="checkbox" id="no-feedback-prompt">
        <label for="no-feedback-prompt">Don't show again</label>
        <mwc-button
          slot="primaryAction"
          dense
          unelevated
          @click=${() => window.open(genFeedbackUrl())}
        >Open Feedback Page</mwc-button>
        <mwc-button slot="secondaryAction" dialogAction="dismiss">Proceed to legacy page</mwc-button>
      </mwc-dialog>
      <div id="build-summary">
        <div id="build-id">
          <span id="build-id-label">Build </span>
          <a href=${getURLForProject(this.builderParam!.project)}>${this.builderParam!.project}</a>
          <span>/</span>
          <span>${this.builderParam!.bucket}</span>
          <span>/</span>
          <a href=${getURLForBuilder(this.builderParam!)}>${this.builderParam!.builder}</a>
          <span>/</span>
          <span>${this.buildNumOrIdParam}</span>
        </div>
        <div class="delimiter"></div>
        <a
          @click=${(e: Event) => {
            const expires = new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toUTCString();
            document.cookie = `showNewBuildPage=false; expires=${expires}; path=/`;
            if (this.configsStore.userConfigs.askForFeedback) {
              this.showFeedbackDialog = true;
              e.preventDefault();
            }
          }}
          href=${this.legacyUrl}
        >Switch to the legacy build page</a>
        <div id="build-status">${this.renderBuildStatus()}</div>
      </div>
      <milo-status-bar
        .components=${[{color: this.statusBarColor, weight: 1}]}
        .loading=${this.buildState.build$.state === PENDING}
      ></milo-status-bar>
      <milo-tab-bar
        .tabs=${this.tabDefs}
        .selectedTabId=${this.appState.selectedTabId}
      ></milo-tab-bar>
      <slot></slot>
    `;
  }

  static styles = css`
    :host {
      height: calc(100vh - var(--header-height));
      display: grid;
      grid-template-rows: repeat(5, auto) 1fr;
    }

    #build-summary {
      background-color: var(--block-background-color);
      padding: 6px 16px;
      font-family: "Google Sans", "Helvetica Neue", sans-serif;
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
    .status.scheduled {
      color: var(--scheduled-color);
    }
    .status.started {
      color: var(--started-color);
    }
    .status.success {
      color: var(--success-color);
    }
    .status.failure {
      color: var(--failure-color);
    }
    .status.infra-failure {
      color: var(--critical-failure-color);
    }
    .status.canceled {
      color: var(--canceled-color);
    }

    milo-tab-bar {
      margin: 0 10px;
      padding-top: 10px;
    }

    #settings-dialog {
      --mdc-dialog-min-width: 600px;
    }
    #default-tab-selector {
      display: inline-block;
      margin-left: 10px;
      padding: .375rem .75rem;
      font-size: 1rem;
      line-height: 1.5;
      background-clip: padding-box;
      border: 1px solid var(--divider-color);
      border-radius: .25rem;
      transition: border-color .15s ease-in-out,box-shadow .15s ease-in-out;
    }
  `;
}

customElement('milo-build-page')(
  provideInvocationState(
    provideBuildState(
      consumeConfigsStore(
        consumeAppState(
          BuildPageElement,
        ),
      ),
    ),
  ),
);
