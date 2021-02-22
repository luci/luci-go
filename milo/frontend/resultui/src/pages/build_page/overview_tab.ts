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

import '../../components/build_step_entry';
import '../../components/code_mirror_editor';
import '../../components/link';
import '../../components/log';
import { AppState, consumeAppState } from '../../context/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import { getBotLink, getBuildbucketLink, getURLForBuild, getURLForGerritChange, getURLForGitilesCommit, getURLForSwarmingTask } from '../../libs/build_utils';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_DISPLAY_MAP } from '../../libs/constants';
import { displayDuration, LONG_TIME_FORMAT } from '../../libs/time_utils';
import { renderMarkdown } from '../../libs/utils';
import { StepExt } from '../../models/step_ext';
import { router } from '../../routes';
import { BuildStatus } from '../../services/buildbucket';

export class OverviewTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;

  @observable.ref private showRetryDialog = false;
  @observable.ref private showCancelDialog = false;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'overview';
    trackEvent(GA_CATEGORIES.OVERVIEW_TAB, GA_ACTIONS.TAB_VISITED);
  }

  private renderActionButtons() {
    const build = this.buildState.build!;

    return html`
      <h3>Actions</h3>
      <div>
        ${build.endTime ?
          html`<mwc-button dense unelevated @click=${() => this.showRetryDialog = true}>Retry Build</mwc-button>` :
          html`<mwc-button dense unelevated @click=${() => this.showCancelDialog = true}>Cancel Build</mwc-button>`}
      </div>
    `;
  }

  private renderCanaryWarning() {
    if (!this.buildState.build?.isCanary) {
      return html``;
    }
    if ([BuildStatus.Failure, BuildStatus.InfraFailure].indexOf(this.buildState.build!.status) === -1) {
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
      id: this.buildState.build!.id,
      summaryMarkdown: reason,
    });
    this.buildState.refresh();
  }

  private async retryBuild() {
    const build = await this.appState.buildsService!.scheduleBuild({
      templateBuildId: this.buildState.build!.id,
    });
    Router.go(getURLForBuild(build));
  }

  private renderSummary() {
    const build = this.buildState.build!;
    if (!build.summaryMarkdown) {
      return html`
        <div id="summary-html" class="${BUILD_STATUS_CLASS_MAP[build.status]}">
          <div id="status">Build ${BUILD_STATUS_DISPLAY_MAP[build.status] || 'status unknown'}</div>
        </div>
      `;
    }

    return html`
      <div id="summary-html" class="${BUILD_STATUS_CLASS_MAP[build.status]}">
        ${renderMarkdown(build.summaryMarkdown)}
      </div>
    `;
  }

  private renderInput() {
    const input = this.buildState.build?.input;
    if (!input) {
      return html``;
    }
    return html`
        <h3>Input</h3>
        ${input.gitilesCommit ? html`
          <div>Revision:
            <a href=${getURLForGitilesCommit(input.gitilesCommit)} target="_blank">${input.gitilesCommit.id}</a>
            ${input.gitilesCommit.position ? `CP #${input.gitilesCommit.position}` : ''}
          </div>
        ` : ''}
        ${(input.gerritChanges || []).map((gc) => html`
          <div>Patch:
            <a href=${getURLForGerritChange(gc)}>
              ${gc.change} (ps #${gc.patchset})
            </a>
          </div>
        `)}
    `;
  }

  private renderInfra() {
    const build = this.buildState.build!;
    const botLink = build.infra?.swarming ? getBotLink(build.infra.swarming) : null;
    return html`
      <h3>Infra</h3>
      <div class="key-value-list">
        <div class="key">Buildbucket ID:</div>
        <div class="value"><milo-link .link=${getBuildbucketLink(CONFIGS.BUILDBUCKET.HOST, build.id)} target="_blank"></div>
        ${build.infra?.swarming ? html`
        <div class="key">Swarming Task:</div>
        <div class="value">${build.infra.swarming.taskId ? html`<a href=${getURLForSwarmingTask(build.infra.swarming)}>${build.infra.swarming.taskId}</a>`: 'N/A'}</div>
        <div class="key">Bot:</div>
        <div class="value">${botLink ? html`<milo-link .link=${botLink} target="_blank"></milo-link>` : 'N/A'}</div>
        ` : ''}
        <div class="key">Recipe:</div>
        <div class="value"><milo-link .link=${build.recipeLink} target="_blank"></milo-link></div>
      </div>
    `;
  }

  private renderSteps() {
    const build = this.buildState.build!;
    const allSteps = (build.rootSteps || [])
      .map((step, i) => [step, i + 1] as [StepExt, number]);
    const nonSucceededSteps = allSteps
      .filter(([step, _stepNum]) => !step.succeededRecursively);
    const scheduledSteps = nonSucceededSteps
      .filter(([step, _stepNum]) => step.status === BuildStatus.Scheduled);
    const runningSteps = nonSucceededSteps
      .filter(([step, _stepNum]) => step.status === BuildStatus.Started);
    const canceledSteps = nonSucceededSteps
      .filter(([step, _stepNum]) => step.status === BuildStatus.Canceled);
    const failedSteps = nonSucceededSteps
      .filter(([step, _stepNum]) => step.failed);
    // If all steps passed, show them all (collapsed by default).
    // Otherwise, if some steps failed or are still running, the user is probably
    // most interested in those, so only show them. Since there are likely only a
    // few non-passing steps, we expand them by default.
    const shownSteps = (nonSucceededSteps.length > 0) ? nonSucceededSteps : allSteps;
    const expandedByDefault = nonSucceededSteps.length > 0;

    return html`
      <div>
        <h3>Steps & Logs
          (<a href=${router.urlForName('build-steps', {
            ...this.buildState.builder,
            build_num_or_id: this.buildState.buildNumOrId!,
          })}>View All</a>)
        </h3>
        <div class="step-summary-line">
          ${this.renderStepSummary(failedSteps.length, canceledSteps.length, scheduledSteps.length, runningSteps.length)}
        </div>
        ${shownSteps.map(([step, stepNum]) => html`
        <milo-build-step-entry
          .expanded=${expandedByDefault}
          .number=${stepNum}
          .step=${step}
          .showDebugLogs=${false}
        ></milo-build-step-entry>
        `) || ''}
      </div>
    `;
  }

  private renderStepSummary(failedSteps: number, canceledSteps: number, scheduledSteps: number, runningSteps: number) {
    if (failedSteps === 0 && scheduledSteps === 0 && runningSteps === 0) {
        return 'All steps succeeded.';
    }
    const messageParts: string[] = [];
    if (failedSteps > 0) {
        messageParts.push(`${failedSteps} step${failedSteps === 1 ? '' : 's'} failed`);
    }
    if (canceledSteps > 0) {
        messageParts.push(`${canceledSteps} step${canceledSteps === 1 ? '' : 's'} canceled`);
    }
    if (scheduledSteps > 0) {
        messageParts.push(`${scheduledSteps} step${scheduledSteps === 1 ? '' : 's'} scheduled`);
    }
    if (runningSteps > 0) {
        messageParts.push(`${runningSteps} step${runningSteps === 1 ? '' : 's'} still running`);
    }
    return messageParts.join(', ') + ':';
  }

  private renderTiming() {
    const build = this.buildState.build!;

    return html`
      <h3>Timing</h3>
      <div class="key-value-list">
        <div class="key">Created:</div>
        <div class="value">${build.createTime.toFormat(LONG_TIME_FORMAT)} (${displayDuration(build.timeSinceCreated)} ago)</div>
        <div class="key">Started:</div>
        <div class="value">${build.startTime && (build.startTime.toFormat(LONG_TIME_FORMAT) + ` (${displayDuration(build.timeSinceStarted!)} ago)`) || 'N/A'}</div>
        <div class="key">Ended:</div>
        <div class="value">${build.endTime && (build.endTime.toFormat(LONG_TIME_FORMAT) + ` (${displayDuration(build.timeSinceEnded!)} ago)`) || 'N/A'}</div>
        <div class="key">Pending:</div>
        <div class="value">${build.pendingDuration && displayDuration(build.pendingDuration) || 'N/A'}${!build.startTime && ' (and counting)' || ''}</div>
        <div class="key">Execution:</div>
        <div class="value">${build.executionDuration && displayDuration(build.executionDuration) || 'N/A'}${!build.endTime && build.startTime && ' (and counting)' || ''}</div>
      </div>
    `;
  }

  private renderTags() {
    const tags = this.buildState.build?.tags;
    if (!tags) {
      return html``;
    }
    return html`
      <h3>Tags</h3>
      <div class="key-value-list">
        ${tags.map((tag) => html`
        <div class="key">${tag.key}:</div>
        <div class="value">${tag.value}</div>
        `)}
      </div>
    `;
  }

  private renderProperties(header: string, properties: {[key: string]: unknown}) {
    const editorOptions = {
      mode: {name: 'javascript', json: true},
      readOnly: true,
      scrollbarStyle: 'null',
      matchBrackets: true,
      lineWrapping: true,
    };

    return html`
      <h3>${header}</h3>
      <milo-code-mirror-editor
        .value=${JSON.stringify(properties, undefined, 2)}
        .options=${{...editorOptions}}
      ></milo-code-mirror-editor>
    `;
  }

  private renderBuildLogs() {
    const logs = this.buildState.build?.output?.logs;
    if (!logs) {
      return html``;
    }
    return html `
      <h3>Build Logs</h3>
      <ul>
        ${logs.map((log) => html`<li><milo-log .log=${log}></li>`)}
      </ul>
    `;
  }

  protected render() {
    const build = this.buildState.build;
    if (!build) {
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
        ${this.renderCanaryWarning()}
        ${this.renderSummary()}
        <div class="first-column">
          <!-- TODO(crbug/1116824): render failed tests -->
          ${this.renderSteps()}
        </div>
        <div class="second-column">
          ${this.renderInput()}
          ${this.renderTiming()}
          ${this.renderInfra()}
          ${this.renderBuildLogs()}
          ${this.renderActionButtons()}
          ${this.renderTags()}
          ${this.renderProperties('Input Properties', build.input.properties)}
          ${this.renderProperties('Output Properties', build.output.properties)}
        </div>
      </div>
    `;
  }

  static styles = css`
    #main {
      margin: 10px 16px;
    }
    @media screen and (min-width: 1500px) {
      #main {
        display: grid;
        grid-template-columns: 1fr 1fr;
        grid-gap: 20px;
      }
      .first-column,
      .second-column {
        overflow: hidden;
      }
    }

    h3 {
      margin-block: 20px 10px;
    }

    #summary-html {
      background-color: var(--block-background-color);
      padding: 0 10px;
      clear: both;
      overflow-wrap: break-word;
      grid-column-end: span 2;
    }
    #summary-html.scheduled {
      border: 1px solid var(--scheduled-color);
      background-color: var(--scheduled-bg-color);
    }
    #summary-html.started {
      border: 1px solid var(--started-color);
      background-color: var(--started-bg-color);
    }
    #summary-html.success {
      border: 1px solid var(--success-color);
      background-color: var(--success-bg-color);
    }
    #summary-html.failure {
      border: 1px solid var(--failure-color);
      background-color: var(--failure-bg-color);
    }
    #summary-html.infra-failure {
      border: 1px solid var(--critical-failure-color);
      background-color: var(--critical-failure-bg-color);
    }
    #summary-html.canceled {
      border: 1px solid var(--canceled-color);
      background-color: var(--canceled-bg-color);
    }
    #summary-html pre {
      white-space: pre-wrap;
      font-size: 12px;
    }
    #summary-html * {
      margin-block: 10px;
    }
    #status {
      font-weight: 500;
    }

    :host > mwc-dialog {
      margin: 0 0;
    }
    #cancel-reason {
      width: 500px;
      height: 200px;
    }

    #canary-warning {
      background-color: var(--warning-color);
      font-weight: 500;
      padding: 5px;
      grid-column-end: span 2;
    }

    .key-value-list {
      display: grid;
      grid-template-columns: auto 1fr;
    }
    .key-value-list div {
      clear: both;
      overflow-wrap: anywhere;
    }
    .key-value-list .value {
      margin-left: 10px;
    }
    .key-value-list>div {
      margin-top: 1px;
      margin-bottom: 1px;
    }

    .step-summary-line {
      margin-bottom: 10px;
    }

    milo-code-mirror-editor {
      min-width: 600px;
      max-width: 1000px;
    }
  `;
}

customElement('milo-overview-tab')(
  consumeBuildState(
    consumeAppState(OverviewTabElement),
  ),
);
