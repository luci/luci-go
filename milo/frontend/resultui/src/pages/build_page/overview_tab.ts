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
import '@material/mwc-button';
import '@material/mwc-dialog';
import '@material/mwc-textarea';
import { TextArea } from '@material/mwc-textarea';
import { Router } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { observable } from 'mobx';

import '../../components/ace_editor';
import '../../components/build_step_entry';
import '../../components/link';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { getBotLink, getURLForGerritChange, getURLForGitilesCommit, getURLForSwarmingTask, stepSucceededRecursive } from '../../libs/build_utils';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_DISPLAY_MAP } from '../../libs/constants';
import { displayTimeDiff, displayTimeDiffOpt, displayTimestamp, displayTimestampOpt } from '../../libs/time_utils';
import { renderMarkdown } from '../../libs/utils';
import { router } from '../../routes';
import { BuildStatus } from '../../services/buildbucket';
import { StepExt } from '../../services/build_page';

export class OverviewTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;

  @observable.ref private showRetryDialog = false;
  @observable.ref private showCancelDialog = false;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'overview';
  }

  private renderStatusTime() {
    const bpd = this.buildState.buildPageData!;

    return html`
      <div id="status">
        Build
        <i class="status ${BUILD_STATUS_CLASS_MAP[bpd.status]}">
          ${BUILD_STATUS_DISPLAY_MAP[bpd.status] || 'unknown status'}
        </i>
        ${(() => { switch (bpd.status) {
        case BuildStatus.Scheduled:
          return `since ${displayTimestamp(bpd.createTime)}`;
        case BuildStatus.Started:
          return `since ${displayTimestamp(bpd.startTime!)}`;
        case BuildStatus.Canceled:
          return `after ${displayTimeDiff(bpd.createTime, bpd.endTime!)} by ${bpd.canceledBy}`;
        case BuildStatus.Failure:
        case BuildStatus.InfraFailure:
        case BuildStatus.Success:
          return `after ${displayTimeDiff(bpd.startTime || bpd.createTime, bpd.endTime!)}`;
        default:
          return '';
        }})()}
        ${bpd.endTime ?
          html`<mwc-button dense unelevated @click=${() => this.showRetryDialog = true}>Retry</mwc-button>` :
          html`<mwc-button dense unelevated @click=${() => this.showCancelDialog = true}>Cancel</mwc-button>`}
      </div>
    `;
  }

  private renderCanaryWarning() {
    if (!this.buildState.isCanary) {
      return html``;
    }
    if ([BuildStatus.Failure, BuildStatus.InfraFailure].indexOf(this.buildState.buildPageData!.status) === -1) {
      return html``;
    }
    return html`
      <div id="canary-warning">
        WARNING: This build ran on a canary version of LUCI. If you suspect it
        failed due to infra, retry the build. Next time it may use the
        non-canary version.
      </div>
    `;
  }

  private async cancelBuild(reason: string) {
    await this.appState.buildsService!.cancelBuild({
      id: this.buildState.buildPageData!.id,
      summaryMarkdown: reason,
    });
    this.buildState.refresh();
  }

  private async retryBuild() {
    const build = await this.appState.buildsService!.scheduleBuild({
      templateBuildId: this.buildState.buildPageData!.id,
    });
    Router.go({
      pathname: router.urlForName('build', {
        project: build.builder.project,
        bucket: build.builder.bucket,
        builder: build.builder.builder,
        build_num_or_id: build.number ? build.number.toString() : `b${build.id}`,
      }),
    });
  }

  private renderSummary() {
    const bpd = this.buildState.buildPageData!;
    if (bpd.summaryMarkdown) {
      return html`
        <div id="summary-html">
          ${renderMarkdown(bpd.summaryMarkdown)}
        </div>
      `;
    }

    if (bpd.summary) {
      return html`
        <div id="summary-html">
          ${bpd.summary.map((summary) => html`${summary}<br>`)}
        </div>
      `;
    }

    return html``;
  }

  private renderInput() {
    const input = this.buildState.buildPageData?.input;
    if (!input) {
      return html``;
    }
    return html`
      <div>
        <h3>Input</h3>
        <table>
          ${input.gitilesCommit ? html`
            <tr>
              <td>Revision:</td>
              <td>
                <a href=${getURLForGitilesCommit(input.gitilesCommit)} target="_blank">${input.gitilesCommit.id}</a>
                ${input.gitilesCommit.position ? `CP #${input.gitilesCommit.position}` : ''}
              </td>
            </tr>
          ` : ''}
          ${(input.gerritChanges || []).map((gc) => html`
            <tr>
              <td>Patch:</td>
              <td>
                <a href=${getURLForGerritChange(gc)}>
                  ${gc.change} (ps #${gc.patchset})
                </a>
              </td>
            </tr>
          `)}
        </table>
      </div>
    `;
  }

  private renderInfra() {
    const bpd = this.buildState.buildPageData!;
    const botLink = bpd.infra?.swarming ? getBotLink(bpd.infra.swarming) : null;
    return html`
      <div>
        <h3>Infra</h3>
        <table>
          <tr><td>Buildbucket ID:</td><td><milo-link .link=${bpd.buildbucketLink} target="_blank"></td></tr>
          ${bpd.infra?.swarming ? html`
          <tr>
            <td>Swarming Task:</td>
            <td>${bpd.infra.swarming.taskId ? html`<a href=${getURLForSwarmingTask(bpd.infra.swarming)}>${bpd.infra.swarming.taskId}</a>`: 'N/A'}</td>
          </tr>
          <tr>
            <td>Bot:</td>
            <td>${botLink ? html`<milo-link .link=${botLink} target="_blank"></milo-link>` : 'N/A'}</td>
          </tr>
          ` : ''}
          <tr><td>Recipe:</td><td><milo-link .link=${bpd.recipeLink} target="_blank"></milo-link></td></tr>
        </table>
      </div>
    `;
  }

  private renderSteps() {
    const bpd = this.buildState.buildPageData!;
    const nonSucceededSteps = (bpd.steps || [])
      .map((step, i) => [step, i + 1] as [StepExt, number])
      .filter(([step, _stepNum]) => !stepSucceededRecursive(step));

    return html`
      <div>
        <h3>Steps & Logs</h3>
        ${nonSucceededSteps.map(([step, stepNum]) => html`
        <milo-build-step-entry
          .expanded=${true}
          .number=${stepNum}
          .step=${step}
          .showDebugLogs=${false}
        ></milo-build-step-entry>
        `) || ''}
        <div class="list-entry">
          ${nonSucceededSteps.length} non-succeeded step(s).
          <a href=${router.urlForName('build-steps', {
            ...this.buildState.builder,
            build_num_or_id: this.buildState.buildNumOrId!,
          })}>View All</a>
        </div>
      </div>
    `;
  }

  private renderTiming() {
    const bpd = this.buildState.buildPageData!;

    return html`
      <div>
        <h3>Timing</h3>
        <table>
          <tr><td>Create:</td><td>${displayTimestamp(bpd.createTime)}</td></tr>
          <tr><td>Start:</td><td>${displayTimestampOpt(bpd.startTime) || 'N/A'}</td></tr>
          <tr><td>End:</td><td>${displayTimestampOpt(bpd.endTime) || 'N/A'}</td></tr>
          <tr><td>Pending:</td><td>${displayTimeDiffOpt(bpd.createTime, bpd.startTime) || 'N/A'}</td></tr>
          <tr><td>Execution:</td><td>${displayTimeDiffOpt(bpd.startTime, bpd.endTime) || 'N/A'}</td></tr>
        </table>
      </div>
    `;
  }

  private renderTags() {
    const tags = this.buildState.buildPageData?.tags;
    if (!tags) {
      return html``;
    }
    return html`
      <div>
        <h3>Tags</h3>
        <table>
          ${tags.map((tag) => html`
          <tr><td>${tag.key}:</td><td>${tag.value}</td></tr>
          `)}
        </table>
      </div>
    `;
  }

  private renderProperties(header: string, properties: {[key: string]: unknown}) {
    const editorOptions = {
      mode: 'ace/mode/json',
      readOnly: true,
      showPrintMargin: false,
      scrollPastEnd: false,
      fontSize: 12,
      maxLines: 50,
    };

    return html`
      <div>
        <h3>${header}</h3>
        <milo-ace-editor
          .options=${{
            ...editorOptions,
            value: JSON.stringify(properties, undefined, 2),
          }}
        ></milo-ace-editor>
      </div>
    `;
  }

  protected render() {
    const bpd = this.buildState.buildPageData;
    if (!bpd) {
      return html``;
    }

    return html`
      <mwc-dialog
        heading="Retry Build"
        ?open=${this.showRetryDialog}
        @closed=${async (event: CustomEvent<{action: string}>) => {
          if (event.detail.action === 'retry') {
            await this.retryBuild();
          }
          this.showRetryDialog = false;
        }}
      >
        <p>Note: this doesn't trigger anything else (e.g. CQ).</p>
        <mwc-button slot="primaryAction" dialogAction="retry" dense unelevated>Retry</mwc-button>
        <mwc-button slot="secondaryAction" dialogAction="dismiss">Dismiss</mwc-button>
      </mwc-dialog>
      <mwc-dialog
        heading="Cancel Build"
        ?open=${this.showCancelDialog}
        @closed=${async (event: CustomEvent<{action: string}>) => {
          if (event.detail.action === 'cancel') {
            const reason = (this.shadowRoot!.getElementById('cancel-reason') as TextArea).value;
            await this.cancelBuild(reason);
          }
          this.showCancelDialog = false;
        }}
      >
        <mwc-textarea id="cancel-reason" label="Reason" required></mwc-textarea>
        <mwc-button slot="primaryAction" dialogAction="cancel" dense unelevated>Cancel</mwc-button>
        <mwc-button slot="secondaryAction" dialogAction="dismiss">Dismiss</mwc-button>
      </mwc-dialog>
      <div id="main">
        <div>
          ${this.renderStatusTime()}
          ${this.renderCanaryWarning()}
          ${this.renderSummary()}
          ${this.renderInput()}
          ${this.renderInfra()}
          ${this.renderTiming()}
          <!-- TODO(crbug/1116824): render failed tests -->
          ${this.renderSteps()}
        </div>
        <div>
          ${this.renderTags()}
          ${this.renderProperties('Input Properties', bpd.input.properties)}
          ${this.renderProperties('Output Properties', bpd.output.properties)}
        </div>
      </div>
    `;
  }

  static styles = css`
    #main > div > * {
      margin: 5px 24px;
    }
    #main > div {
      float: left;
      min-width: 600px;
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

    :host > mwc-dialog {
      margin: 0 0;
    }
    #cancel-reason {
      width: 500px;
      height: 200px;
    }
    mwc-button {
      transform: scale(0.8);
      vertical-align: middle;
    }

    #canary-warning {
      background-color: var(--warning-color);
      font-weight: 500;
    }

    #summary-html {
      background-color: var(--block-background-color);
      padding: 5px;
    }

    .list-entry {
      margin-top: 5px;
    }

    milo-ace-editor {
      width: 800px;
    }
  `;
}

customElement('milo-overview-tab')(
  consumeBuildState(
    consumeAppState(OverviewTabElement),
  ),
);
