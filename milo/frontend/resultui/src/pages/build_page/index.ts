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
import { autorun, computed, observable } from 'mobx';

import '../../components/status_bar';
import '../../components/tab_bar';
import { TabDef } from '../../components/tab_bar';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { consumeInvocationState, InvocationState } from '../../context/invocation_state/invocation_state';
import { getURLForBuilder, getURLForProject } from '../../libs/build_utils';
import { displayTimeDiff, displayTimestamp } from '../../libs/time_utils';
import { NOT_FOUND_URL, router } from '../../routes';
import { BuilderID, BuildStatus } from '../../services/buildbucket';

export const STATUS_DISPLAY_MAP = {
  [BuildStatus.Scheduled]: 'scheduled',
  [BuildStatus.Started]: 'started',
  [BuildStatus.Success]: 'succeeded',
  [BuildStatus.Failure]: 'failed',
  [BuildStatus.InfraFailure]: 'infra failed',
  [BuildStatus.Canceled]: 'canceled',
};

export const STATUS_CLASS_MAP = {
  [BuildStatus.Scheduled]: 'scheduled',
  [BuildStatus.Started]: 'started',
  [BuildStatus.Success]: 'success',
  [BuildStatus.Failure]: 'failure',
  [BuildStatus.InfraFailure]: 'infra-failure',
  [BuildStatus.Canceled]: 'canceled',
};

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
    return;
  }

  private disposers: Array<() => void> = [];

  connectedCallback() {
    super.connectedCallback();
    this.buildState.builder = this.builder;
    this.buildState.buildNumOrId = this.buildNumOrId;

    this.disposers.push(autorun(
      () => this.invocationState.invocationId = this.buildState.buildPageData
        ?.infra?.resultdb.invocation.slice('invocations/'.length) || '',
    ));
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
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
      {
        id: 'test-results',
        label: 'Test Results',
        href: router.urlForName('build-test-results', params),
      },
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

  private renderBuildStatus() {
    const bpd = this.buildState.buildPageData;
    if (!bpd) {
      return html``;
    }
    return html`
      <i class="status ${STATUS_CLASS_MAP[bpd.status]}">
        ${STATUS_DISPLAY_MAP[bpd.status] || 'unknown status'}
      </i>
      ${(() => { switch (bpd.status) {
      case BuildStatus.Scheduled:
        return `since ${displayTimestamp(bpd.create_time)}`;
      case BuildStatus.Started:
        return `since ${displayTimestamp(bpd.start_time!)}`;
      case BuildStatus.Canceled:
        return `after ${displayTimeDiff(bpd.create_time, bpd.end_time!)}`;
      case BuildStatus.Failure:
      case BuildStatus.InfraFailure:
      case BuildStatus.Success:
        return `after ${displayTimeDiff(bpd.start_time || bpd.create_time, bpd.end_time!)}`;
      default:
        return '';
      }})()}
    `;
  }

  protected render() {
    return html`
      <div id="build-summary">
        <div id="build-id">
          <span id="build-id-label">Build</span>
          <a href=${getURLForProject(this.builder.project)}>${this.builder.project}</a> /
          <span>${this.builder.bucket}</span> /
          <a href=${getURLForBuilder(this.builder)}>${this.builder.builder}</a> /
          <span>${this.buildNumOrId}</span>
        </div>
        <div id="build-status">${this.renderBuildStatus()}</div>
      </div>
      <milo-status-bar
        .components=${[{color: '#007bff', weight: 1}]}
        .loading=${this.buildState.buildPageDataReq.state === 'pending'}
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
      background-color: rgb(248, 249, 250);
      padding: 6px 16px;
      font-family: "Google Sans", "Helvetica Neue", sans-serif;
      font-size: 14px;
      display: flex;
    }

    #build-id {
      flex: 0 auto;
    }

    #build-id-label {
      color: rgb(95, 99, 104);
    }

    #build-status {
      margin-left: auto;
      flex: 0 auto;
    }
    #status {
      font-weight: 500;
    }
    .status.scheduled {
      color: #6c757d;
    }
    .status.started {
      color: #ffc107;
    }
    .status.success {
      color: #28a745;
    }
    .status.failure {
      color: #dc3545;
    }
    .status.infra-failure {
      color: #6f42c1;
    }
    .status.canceled {
      color: #6c757d;
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
