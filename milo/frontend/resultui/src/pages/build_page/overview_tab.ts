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
import '@material/mwc-textarea';
import { MobxLitElement } from '@adobe/lit-mobx';
import { TextArea } from '@material/mwc-textarea';
import { Router } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { observable } from 'mobx';

import '../../components/build_tag_row';
import '../../components/build_step_entry';
import '../../components/link';
import '../../components/log';
import '../../components/property_viewer';
import '../../components/timestamp';
import { AppState, consumeAppState } from '../../context/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state';
import { consumeConfigsStore, UserConfigsStore } from '../../context/user_configs';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import {
  getBotLink,
  getBuildbucketLink,
  getURLForGerritChange,
  getURLForGitilesCommit,
  getURLForSwarmingTask,
  getURLPathForBuild,
} from '../../libs/build_utils';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_DISPLAY_MAP } from '../../libs/constants';
import { renderMarkdown } from '../../libs/markdown_utils';
import { sanitizeHTML } from '../../libs/sanitize_html';
import { displayDuration } from '../../libs/time_utils';
import { StepExt } from '../../models/step_ext';
import { router } from '../../routes';
import { BuildStatus } from '../../services/buildbucket';

@customElement('milo-overview-tab')
@consumeBuildState
@consumeConfigsStore
@consumeAppState
export class OverviewTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref configsStore!: UserConfigsStore;
  @observable.ref buildState!: BuildState;

  @observable.ref private showRetryDialog = false;
  @observable.ref private showCancelDialog = false;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'overview';
    trackEvent(GA_CATEGORIES.OVERVIEW_TAB, GA_ACTIONS.TAB_VISITED, window.location.href);
  }

  private renderActionButtons() {
    const build = this.buildState.build!;

    const canRetry = this.buildState.permittedActions.has('ADD_BUILD');
    const canCancel = this.buildState.permittedActions.has('CANCEL_BUILD');

    if (build.endTime) {
      return html`
        <h3>Actions</h3>
        <div title=${canRetry ? '' : 'You have no permission to retry this build.'}>
          <mwc-button dense unelevated @click=${() => (this.showRetryDialog = true)} ?disabled=${!canRetry}>
            Retry Build
          </mwc-button>
        </div>
      `;
    }

    return html`
      <h3>Actions</h3>
      <div title=${canCancel ? '' : 'You have no permission to cancel this build.'}>
        <mwc-button dense unelevated @click=${() => (this.showCancelDialog = true)} ?disabled=${!canCancel}>
          Cancel Build
        </mwc-button>
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
      <div id="canary-warning" class="wide">
        WARNING: This build ran on a canary version of LUCI. If you suspect it failed due to infra, retry the build.
        Next time it may use the non-canary version.
      </div>
    `;
  }

  private async cancelBuild(reason: string) {
    await this.appState.buildsService!.cancelBuild({
      id: this.buildState.build!.id,
      summaryMarkdown: reason,
    });
    this.appState.refresh();
  }

  private async retryBuild() {
    const build = await this.appState.buildsService!.scheduleBuild({
      templateBuildId: this.buildState.build!.id,
    });
    Router.go(getURLPathForBuild(build));
  }

  private renderSummary() {
    const build = this.buildState.build!;
    if (!build.summaryMarkdown) {
      return html`
        <div id="summary-html" class="wide ${BUILD_STATUS_CLASS_MAP[build.status]}">
          <div id="status">Build ${BUILD_STATUS_DISPLAY_MAP[build.status] || 'status unknown'}</div>
        </div>
      `;
    }

    return html`
      <div id="summary-html" class="wide ${BUILD_STATUS_CLASS_MAP[build.status]}">
        ${renderMarkdown(build.summaryMarkdown)}
      </div>
    `;
  }

  private renderBuilderDescription() {
    const descriptionHtml = this.buildState.builder?.config.descriptionHtml;
    if (!descriptionHtml) {
      return html``;
    }
    return html`
      <h3 class="wide">Builder Info</h3>
      <div id="builder-description" class="wide">${sanitizeHTML(descriptionHtml)}</div>
    `;
  }

  private renderInput() {
    const input = this.buildState.build?.input;
    if (!input) {
      return html``;
    }
    return html`
      <h3>Input</h3>
      <table>
        ${input.gitilesCommit
          ? html`
              <tr>
                <td>Revision:</td>
                <td>
                  <a href=${getURLForGitilesCommit(input.gitilesCommit)} target="_blank">${input.gitilesCommit.id}</a>
                  ${input.gitilesCommit.position ? `CP #${input.gitilesCommit.position}` : ''}
                </td>
              </tr>
            `
          : ''}
        ${(input.gerritChanges || []).map(
          (gc) => html`
            <tr>
              <td>Patch:</td>
              <td>
                <a href=${getURLForGerritChange(gc)}> ${gc.change} (ps #${gc.patchset}) </a>
              </td>
            </tr>
          `
        )}
      </table>
    `;
  }

  private renderInfra() {
    const build = this.buildState.build!;
    const botLink = build.infra?.swarming ? getBotLink(build.infra.swarming) : null;
    return html`
      <h3>Infra</h3>
      <table>
        <tr>
          <td>Buildbucket ID:</td>
          <td><milo-link .link=${getBuildbucketLink(CONFIGS.BUILDBUCKET.HOST, build.id)} target="_blank"></td>
        </tr>
        ${
          build.infra?.swarming
            ? html`
                <tr>
                  <td>Swarming Task:</td>
                  <td>
                    ${!build.infra.swarming.taskId
                      ? 'N/A'
                      : html`
                          <a href=${getURLForSwarmingTask(build.infra.swarming.hostname, build.infra.swarming.taskId)}>
                            ${build.infra.swarming.taskId}
                          </a>
                        `}
                  </td>
                </tr>
                <tr>
                  <td>Bot:</td>
                  <td>${botLink ? html`<milo-link .link=${botLink} target="_blank"></milo-link>` : 'N/A'}</td>
                </tr>
              `
            : ''
        }
        <tr>
          <td>Recipe:</td>
          <td><milo-link .link=${build.recipeLink} target="_blank"></milo-link></td>
        </tr>
      </table>
    `;
  }

  private renderSteps() {
    const build = this.buildState.build!;
    const allSteps = (build.rootSteps || []).map((step, i) => [step, i + 1] as [StepExt, number]);
    const importantSteps = allSteps.filter(
      ([step, _stepNum]) => !step.succeededRecursively || this.configsStore.stepIsPinned(step.name)
    );
    const scheduledSteps = importantSteps.filter(([step, _stepNum]) => step.status === BuildStatus.Scheduled);
    const runningSteps = importantSteps.filter(([step, _stepNum]) => step.status === BuildStatus.Started);
    const canceledSteps = importantSteps.filter(([step, _stepNum]) => step.status === BuildStatus.Canceled);
    const failedSteps = importantSteps.filter(([step, _stepNum]) => step.failed);

    const shownSteps = importantSteps.length > 0 ? importantSteps : allSteps;

    return html`
      <div>
        <h3>
          Steps & Logs (<a
            href=${router.urlForName('build-steps', {
              ...this.buildState.builderIdParam,
              build_num_or_id: this.buildState.buildNumOrId!,
            })}
            >View All</a
          >)
        </h3>
        <div class="step-summary-line">
          ${this.renderStepSummary(
            failedSteps.length,
            canceledSteps.length,
            scheduledSteps.length,
            runningSteps.length
          )}
        </div>
        ${shownSteps.map(
          ([step, stepNum]) => html`
            <milo-build-step-entry .number=${stepNum} .step=${step} .showDebugLogs=${false}></milo-build-step-entry>
          `
        ) || ''}
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
      <table>
        <tr>
          <td>Created:</td>
          <td>
            <milo-timestamp .datetime=${build.createTime}></milo-timestamp>
          </td>
        </tr>
        <tr>
          <td>Started:</td>
          <td>${build.startTime ? html`<milo-timestamp .datetime=${build.startTime}></milo-timestamp>` : 'N/A'}</td>
        </tr>
        <tr>
          <td>Ended:</td>
          <td>${build.endTime ? html`<milo-timestamp .datetime=${build.endTime}></milo-timestamp>` : 'N/A'}</td>
        </tr>
        <tr>
          <td>Pending:</td>
          <td>
            ${(build.pendingDuration && displayDuration(build.pendingDuration)) || 'N/A'}${(!build.startTime &&
              ' (and counting)') ||
            ''}
          </td>
        </tr>
        <tr>
          <td>Execution:</td>
          <td>
            ${(build.executionDuration && displayDuration(build.executionDuration)) || 'N/A'}${(!build.endTime &&
              build.startTime &&
              ' (and counting)') ||
            ''}
          </td>
        </tr>
      </table>
    `;
  }

  private renderTags() {
    const tags = this.buildState.build?.tags;
    if (!tags) {
      return html``;
    }
    return html`
      <h3>Tags</h3>
      <table>
        ${tags.map((tag) => html` <milo-build-tag-row .key=${tag.key} .value=${tag.value}> </milo-build-tag-row> `)}
      </table>
    `;
  }

  private renderExperiments() {
    const experiments = this.buildState.build?.input?.experiments;
    if (!experiments) {
      return html``;
    }
    return html`
      <h3>Enabled Experiments</h3>
      <ul>
        ${experiments.map((exp) => html`<li>${exp}</li>`)}
      </ul>
    `;
  }

  private renderBuildLogs() {
    const logs = this.buildState.build?.output?.logs;
    if (!logs) {
      return html``;
    }
    return html`
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
        @closed=${async (event: CustomEvent<{ action: string }>) => {
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
        @closed=${async (event: CustomEvent<{ action: string }>) => {
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
        <div class="first-column">
          ${this.renderCanaryWarning()} ${this.renderSummary()} ${this.renderSteps()}
          <!-- TODO(crbug/1116824): render failed tests -->
        </div>
        <div class="second-column">
          ${this.renderBuilderDescription()} ${this.renderInput()} ${this.renderTiming()} ${this.renderInfra()}
          ${this.renderBuildLogs()} ${this.renderActionButtons()} ${this.renderTags()} ${this.renderExperiments()}
          <h3>Input Properties</h3>
          <milo-property-viewer
            .properties=${build.input.properties}
            .propLineFoldTime=${this.configsStore.userConfigs.inputPropLineFoldTime}
            .saveFoldTime=${() => this.configsStore.save()}
          ></milo-property-viewer>
          <h3>Output Properties</h3>
          <milo-property-viewer
            .properties=${build.output.properties}
            .propLineFoldTime=${this.configsStore.userConfigs.outputPropLineFoldTime}
            .saveFoldTime=${() => this.configsStore.save()}
          ></milo-property-viewer>
        </div>
      </div>
    `;
  }

  static styles = css`
    #main {
      margin: 10px 16px;
    }
    @media screen and (min-width: 1300px) {
      #main {
        display: grid;
        grid-template-columns: auto 20px auto;
      }
      .second-column > h3:first-child {
        margin-block: 5px 10px;
      }
    }
    .first-column {
      overflow: hidden;
      min-width: 50vw;
      grid-column: 1;
    }
    .second-column {
      overflow: hidden;
      min-width: 30vw;
      grid-column: 3;
    }

    h3 {
      margin-block: 15px 10px;
    }

    .wide {
      grid-column-end: span 3;
    }

    #summary-html {
      background-color: var(--block-background-color);
      padding: 0 10px;
      clear: both;
      overflow-wrap: break-word;
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
    }

    td:nth-child(2) {
      clear: both;
      overflow-wrap: anywhere;
    }

    .step-summary-line {
      margin-bottom: 10px;
    }

    milo-property-viewer {
      min-width: 600px;
      max-width: 1000px;
    }
  `;
}
