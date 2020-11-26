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
import '@material/mwc-icon';
import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { autorun, computed, observable, reaction } from 'mobx';

import '../../components/status_bar';
import '../../components/tab_bar';
import { TabDef } from '../../components/tab_bar';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { consumeInvocationState, InvocationState } from '../../context/invocation_state/invocation_state';
import { getLegacyURLForBuild, getURLForBuilder, getURLForProject } from '../../libs/build_utils';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_COLOR_MAP, BUILD_STATUS_DISPLAY_MAP } from '../../libs/constants';
import { displayDuration, LONG_TIME_FORMAT } from '../../libs/time_utils';
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

/**
 * Main build page.
 * Reads project, bucket, builder and build from URL params.
 * If any of the parameters are not provided, redirects to '/not-found'.
 * If build is not a number, shows an error.
 */
export class BuildPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;
  @observable.ref invocationState!: InvocationState;

  private builder!: BuilderID;
  private buildNumOrId = '';

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const project = location.params['project'];
    const bucket = location.params['bucket'];
    const builder = location.params['builder'];
    const buildNumOrId = location.params['build_num_or_id'];
    if ([project, bucket, builder, buildNumOrId].some((param) => typeof param !== 'string')) {
      return cmd.redirect(NOT_FOUND_URL);
    }

    this.builder = {
      project: project as string,
      bucket: bucket as string,
      builder: builder as string,
    };
    this.buildNumOrId = buildNumOrId as string;

    document.title = `${builder} ${buildNumOrId}`;
    return;
  }

  private disposers: Array<() => void> = [];

  @computed private get faviconUrl() {
    if (this.buildState.build) {
      return `/static/common/favicon/${STATUS_FAVICON_MAP[this.buildState.build.status]}-32.png`;
    }
    return `/static/common/favicon/milo-32.png`;
  }

  connectedCallback() {
    super.connectedCallback();
    this.buildState.builder = this.builder;
    this.buildState.buildNumOrId = this.buildNumOrId;

    this.disposers.push(autorun(() => {
      const bpd = this.buildState.build;
      if (!bpd) {
        return;
      }
      this.invocationState.invocationId = bpd.infra?.resultdb?.invocation
        ?.slice('invocations/'.length) || '';
      this.invocationState.initialized = true;
    }));

    this.disposers.push((reaction(
      () => this.faviconUrl,
      (faviconUrl) => document.getElementById('favicon')?.setAttribute('href', faviconUrl),
      {fireImmediately: true},
    )));
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
  }

  @computed get hasInvocation() {
    return this.invocationState.invocationId !== '';
  }

  @computed get tabDefs(): TabDef[] {
    const params = {
      'project': this.builder.project,
      'bucket': this.builder.bucket,
      'builder': this.builder.builder,
      'build_num_or_id': this.buildNumOrId,
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
    return html`
      <div id="build-summary">
        <div id="build-id">
          <span id="build-id-label">Build </span>
          <a href=${getURLForProject(this.builder.project)}>${this.builder.project}</a>
          <span>/</span>
          <span>${this.builder.bucket}</span>
          <span>/</span>
          <a href=${getURLForBuilder(this.builder)}>${this.builder.builder}</a>
          <span>/</span>
          <span>${this.buildNumOrId}</span>
        </div>
        <div class="delimiter"></div>
        <a
          @click=${() => {
            const expires = new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toUTCString();
            document.cookie = `showNewBuildPage=false; expires=${expires}; path=/`;
          }}
          href=${getLegacyURLForBuild(this.builder, this.buildNumOrId)}
        >Switch to the legacy build page</a>
        <div id="build-status">${this.renderBuildStatus()}</div>
      </div>
      <milo-status-bar
        .components=${[{color: this.statusBarColor, weight: 1}]}
        .loading=${this.buildState.buildReq.state === 'pending'}
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
      grid-template-rows: repeat(3, auto) 1fr;
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
  `;
}

customElement('milo-build-page')(
  consumeInvocationState(
    consumeBuildState(
      consumeAppState(
        BuildPageElement,
      ),
    ),
  ),
);
