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
import { css, customElement, html, TemplateResult } from 'lit-element';
import { computed, makeObservable, observable } from 'mobx';

import '../../components/build_tag_row';
import '../../components/expandable_entry';
import '../../components/link';
import '../../components/log';
import '../../components/property_viewer';
import '../../components/relative_timestamp';
import '../../components/timestamp';
import '../test_results_tab/test_variants_table/test_variant_entry';
import './steps_tab/step_list';
import { BuildState, consumeBuildState } from '../../context/build_state';
import { consumeInvocationState, InvocationState } from '../../context/invocation_state';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import {
  getBotLink,
  getBuildbucketLink,
  getCipdLink,
  getURLForGerritChange,
  getURLForGitilesCommit,
  getURLForSwarmingTask,
  getURLPathForBuild,
} from '../../libs/build_utils';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_DISPLAY_MAP } from '../../libs/constants';
import { consumer } from '../../libs/context';
import { renderMarkdown } from '../../libs/markdown_utils';
import { sanitizeHTML } from '../../libs/sanitize_html';
import { displayDuration } from '../../libs/time_utils';
import { router } from '../../routes';
import { ADD_BUILD_PERM, BuildStatus, CANCEL_BUILD_PERM, GitilesCommit } from '../../services/buildbucket';
import { createTVPropGetter, getPropKeyLabel } from '../../services/resultdb';
import { consumeStore, StoreInstance } from '../../store';
import colorClasses from '../../styles/color_classes.css';
import commonStyle from '../../styles/common_style.css';

const MAX_DISPLAYED_UNEXPECTED_TESTS = 10;

@customElement('milo-overview-tab')
@consumer
export class OverviewTabElement extends MobxLitElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref
  @consumeBuildState()
  buildState!: BuildState;

  @observable.ref
  @consumeInvocationState()
  invocationState!: InvocationState;

  @observable.ref private showRetryDialog = false;
  @observable.ref private showCancelDialog = false;

  @computed private get columnGetters() {
    return this.invocationState.columnKeys.map((col) => createTVPropGetter(col));
  }

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();
    this.store.setSelectedTabId('overview');
    trackEvent(GA_CATEGORIES.OVERVIEW_TAB, GA_ACTIONS.TAB_VISITED, window.location.href);
  }

  private renderActionButtons() {
    const build = this.buildState.build!;

    const canRetry = this.buildState.permittedActions[ADD_BUILD_PERM] ?? false;

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

    const canCancel = build.cancelTime === null && this.buildState.permittedActions[CANCEL_BUILD_PERM];
    let tooltip = '';
    if (!canCancel) {
      tooltip =
        build.cancelTime === null
          ? 'You have no permission to cancel this build.'
          : 'The build is already scheduled to be canceled.';
    }

    return html`
      <h3>Actions</h3>
      <div title=${tooltip}>
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
      <div id="canary-warning" class="warning">
        WARNING: This build ran on a canary version of LUCI. If you suspect it failed due to infra, retry the build.
        Next time it may use the non-canary version.
      </div>
    `;
  }

  private renderCancelSchedule() {
    const build = this.buildState.build!;
    if (!build?.cancelTime || build?.endTime) {
      return html``;
    }

    // We have gracePeriod since Feb 2021. It's safe to use ! since all new
    // (not-terminated) builds should have gracePeriod defined.
    // TODO(weiweilin): refresh the build automatically once we reached
    // scheduledCancelTime.
    const scheduledCancelTime = build
      .cancelTime!.plus(build.gracePeriod!)
      // Add min_update_interval (currently always 30s).
      // TODO(crbug/1299302): read min_update_interval from buildbucket.
      .plus({ seconds: 30 });

    return html`
      <div id="scheduled-cancel">
        This build was scheduled to be canceled by ${build.canceledBy || 'unknown'}
        <milo-relative-timestamp .timestamp=${scheduledCancelTime}></milo-relative-timestamp>.
      </div>
    `;
  }

  private async cancelBuild(reason: string) {
    await this.store.services.builds!.cancelBuild({
      id: this.buildState.build!.id,
      summaryMarkdown: reason,
    });
    this.store.refresh();
  }

  private async retryBuild() {
    const build = await this.store.services.builds!.scheduleBuild({
      templateBuildId: this.buildState.build!.id,
    });
    Router.go(getURLPathForBuild(build));
  }

  private renderSummary() {
    const build = this.buildState.build!;
    if (!build.summaryMarkdown) {
      return html`
        <div id="summary-html" class="${BUILD_STATUS_CLASS_MAP[build.status]}-bg">
          <div id="status">Build ${BUILD_STATUS_DISPLAY_MAP[build.status] || 'status unknown'}</div>
        </div>
      `;
    }

    return html`
      <div id="summary-html" class="${BUILD_STATUS_CLASS_MAP[build.status]}-bg">
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
      <h3>Builder Info</h3>
      <div id="builder-description">${sanitizeHTML(descriptionHtml)}</div>
    `;
  }

  private renderRevision(gitilesCommit: GitilesCommit) {
    return html`
      <tr>
        <td>Revision:</td>
        <td>
          <a href=${getURLForGitilesCommit(gitilesCommit)} target="_blank">${gitilesCommit.id}</a>
          ${gitilesCommit.position ? `CP #${gitilesCommit.position}` : ''}
        </td>
      </tr>
    `;
  }

  private renderInput() {
    const input = this.buildState.build?.input;
    if (!input?.gitilesCommit && !input?.gerritChanges) {
      return html``;
    }
    return html`
      <h3>Input</h3>
      <table>
        ${input.gitilesCommit ? this.renderRevision(input.gitilesCommit) : ''}
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

  private renderOutput() {
    const output = this.buildState.build?.output;
    if (!output?.gitilesCommit) {
      return html``;
    }
    return html`
      <h3>Output</h3>
      <table>
        ${this.renderRevision(output.gitilesCommit)}
      </table>
    `;
  }

  private renderBuildPackages() {
    const experiments = this.buildState.build?.input?.experiments;
    const agent = this.buildState.build?.infra?.buildbucket?.agent;
    if (!experiments?.includes('luci.buildbucket.agent.cipd_installation') || !agent) {
      return html``;
    }

    const inputCipdHtml = [];
    for (const dir in agent.input.data) {
      const specs = agent.input.data[dir]?.cipd.specs;
      if (!specs?.length) {
        continue;
      }
      if (dir) {
        inputCipdHtml.push(
          html`<tr>
            <td colspan="2"><br />@Subdir ${dir}</td>
          </tr>`
        );
      }
      for (const pkgSpec of specs) {
        inputCipdHtml.push(
          html`<tr>
            <td>${pkgSpec.package}</td>
            <td>${pkgSpec.version}</td>
          </tr>`
        );
      }
    }

    const resolvedCipdHtml = [];
    for (const dir in agent.output?.resolvedData) {
      const specs = agent.output?.resolvedData[dir]?.cipd.specs;
      if (!specs?.length) {
        continue;
      }
      if (dir) {
        resolvedCipdHtml.push(
          html`<tr>
            <td colspan="2"><br />@Subdir ${dir}</td>
          </tr>`
        );
      }
      for (const pkgSpec of specs) {
        resolvedCipdHtml.push(
          html`<tr>
            <td>${pkgSpec.package}</td>
            <td><milo-link .link=${getCipdLink(pkgSpec.package, pkgSpec.version)} target="_blank"></milo-link></td>
          </tr>`
        );
      }
    }

    return html`
      <h3>Build Packages Info</h3>
      <table id="build-pkgs-table">
        <tbody>
          ${agent.output?.status === BuildStatus.Success
            ? ''
            : html`
                <tr>
                  <td>Status</td>
                  ${agent.output?.status}
                  <td><br /></td>
                </tr>
                <tr>
                  <td>Summary</td>
                  ${agent.output?.summaryHtml}
                  <td><br /></td>
                </tr>
              `}
          <tr>
            <td>Agent Platform</td>
            <td>${agent.output?.agentPlatform || 'N/A'}</td>
          </tr>

          <tr>
            <td>Download Duration</td>
            <td>${agent.output?.totalDuration || 'N/A'}</td>
          </tr>
          <tr>
            <td>Requested CIPD Manifest</td>
            <td>
              <milo-expandable-entry .contentRuler="none" .expanded=${false}>
                <div slot="content" class="nav-scrollbar">
                  <table class="nested-manifest-table">
                    <tr>
                      <td>$ServiceURL https://chrome-infra-packages.appspot.com/<br /></td>
                    </tr>
                    ${inputCipdHtml}
                  </table>
                </div>
              </milo-expandable-entry>
            </td>
          </tr>

          ${agent.output
            ? html`<tr>
                <td>Resolved CIPD Manifest</td>
                <td>
                  <milo-expandable-entry .contentRuler="none" .expanded=${false}>
                    <div slot="content" class="nav-scrollbar">
                      <table class="nested-manifest-table">
                        <tr>
                          <td>$ServiceURL https://chrome-infra-packages.appspot.com/</td>
                        </tr>
                        ${resolvedCipdHtml}
                      </table>
                    </div>
                  </milo-expandable-entry>
                </td>
              </tr>`
            : ''}
        </tbody>
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
                <tr>
                  <td>Service Account:</td>
                  <td>${build.infra.swarming.taskServiceAccount}</td>
                </tr>
                <tr>
                  <td>Ancestor Builds:</td>
                  <td>
                    ${build.ancestorIds.length === 0
                      ? 'no ancestor builds'
                      : build.ancestorIds.map((id) => html`<a href="/b/${id}">${id}</a> `)}
                  </td>
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

  private renderFailedTests() {
    if (!this.invocationState.hasInvocation) {
      return;
    }

    const testLoader = this.invocationState.testLoader;
    const testsTabUrl = router.urlForName('build-test-results', {
      ...this.buildState.builderIdParam!,
      build_num_or_id: this.buildState.buildNumOrIdParam!,
    });

    // Overview tab is more crowded than the test results tab.
    // Hide all additional columns.
    const columnWidths = '24px ' + this.invocationState.columnWidths.map(() => '0').join(' ') + ' 1fr';

    return html`
      <h3>Failed Tests (<a href=${testsTabUrl}>View All Tests</a>)</h3>
      <div id="failed-tests-section-body">
        ${testLoader?.firstPageLoaded
          ? html`
              <div style="--tvt-columns: ${columnWidths}">${this.renderFailedTestList()}</div>
              <div id="failed-test-count">
                ${testLoader.unfilteredUnexpectedVariantsCount === 0
                  ? 'No failed tests.'
                  : html`Showing
                      ${Math.min(testLoader.unfilteredUnexpectedVariantsCount, MAX_DISPLAYED_UNEXPECTED_TESTS)} /
                      ${testLoader.unfilteredUnexpectedVariantsCount} failed tests.
                      <a href=${testsTabUrl}>[view all]</a>`}
              </div>
            `
          : html`<div id="loading-spinner">Loading <milo-dot-spinner></milo-dot-spinner></div>`}
      </div>
    `;
  }

  private renderFailedTestList() {
    const testLoader = this.invocationState.testLoader!;
    const groupDefs = this.invocationState.groupers
      .filter(([key]) => key !== 'status')
      .map(([key, getter]) => [getPropKeyLabel(key), getter] as [string, typeof getter]);

    let remaining = MAX_DISPLAYED_UNEXPECTED_TESTS;
    const htmlTemplates: TemplateResult[] = [];
    for (const group of testLoader.groupedUnfilteredUnexpectedVariants) {
      if (remaining === 0) {
        break;
      }
      if (groupDefs.length !== 0) {
        htmlTemplates.push(html`<h4>${groupDefs.map(([label, getter]) => html`${label}: ${getter(group[0])}`)}</h4>`);
      }
      for (const testVariant of group) {
        if (remaining === 0) {
          break;
        }
        remaining--;
        htmlTemplates.push(html`
          <milo-test-variant-entry
            .variant=${testVariant}
            .columnGetters=${this.columnGetters}
          ></milo-test-variant-entry>
        `);
      }
    }

    return htmlTemplates;
  }

  private renderSteps() {
    const stepsUrl = router.urlForName('build-steps', {
      ...this.buildState.builderIdParam,
      build_num_or_id: this.buildState.buildNumOrIdParam!,
    });
    return html`
      <div>
        <h3>Steps & Logs (<a href=${stepsUrl}>View in Steps Tab</a>)</h3>
        <div id="step-config">
          Show:
          <input
            id="succeeded"
            type="checkbox"
            ?checked=${this.store.userConfig.build.steps.showSucceededSteps}
            @change=${(e: MouseEvent) => {
              this.store.userConfig.build.steps.setShowSucceededSteps((e.target as HTMLInputElement).checked);
            }}
          />
          <label for="succeeded" style="color: var(--success-color);">Succeeded Steps</label>
          <span>&nbsp;</span>
          <input
            id="debug-logs"
            type="checkbox"
            ?checked=${this.store.userConfig.build.steps.showDebugLogs}
            @change=${(e: MouseEvent) => {
              this.store.userConfig.build.steps.setShowDebugLogs((e.target as HTMLInputElement).checked);
            }}
          />
          <label id="debug-logs-label" for="debug-logs">Debug Logs</label>
          <input
            id="expand-by-default"
            type="checkbox"
            ?checked=${this.store.userConfig.build.steps.expandSucceededByDefault}
            @change=${(e: MouseEvent) => {
              this.store.userConfig.build.steps.setExpandSucceededByDefault((e.target as HTMLInputElement).checked);
            }}
          />
          <label for="expand-by-default">Expand by default</label>
        </div>
        <milo-bp-step-list></milo-bp-step-list>
      </div>
    `;
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
            ${displayDuration(build.pendingDuration)} ${build.isPending ? '(and counting)' : ''}
            ${build.exceededSchedulingTimeout ? html`<span class="warning">(exceeded timeout)</span>` : ''}
            <mwc-icon
              class="inline-icon"
              title="Maximum pending duration: ${build.schedulingTimeout
                ? displayDuration(build.schedulingTimeout)
                : 'N/A'}"
              >info</mwc-icon
            >
          </td>
        </tr>
        <tr>
          <td>Execution:</td>
          <td>
            ${build.executionDuration ? displayDuration(build.executionDuration) : 'N/A'}
            ${build.isExecuting ? '(and counting)' : ''}
            ${build.exceededExecutionTimeout ? html`<span class="warning">(exceeded timeout)</span>` : ''}
            <mwc-icon
              class="inline-icon"
              title="Maximum execution duration: ${build.executionTimeout
                ? displayDuration(build.executionTimeout)
                : 'N/A'}"
              >info</mwc-icon
            >
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
        <mwc-button slot="primaryAction" dialogAction="cancel" dense unelevated>Confirm</mwc-button>
        <mwc-button slot="secondaryAction" dialogAction="dismiss">Dismiss</mwc-button>
      </mwc-dialog>
      <div id="main">
        <div class="first-column">
          ${this.renderCanaryWarning()}${this.renderCancelSchedule()}
          ${this.renderSummary()}${this.renderFailedTests()}${this.renderSteps()}
        </div>
        <div class="second-column">
          ${this.renderBuilderDescription()} ${this.renderInput()} ${this.renderOutput()} ${this.renderInfra()}
          ${this.renderTiming()} ${this.renderBuildLogs()} ${this.renderActionButtons()} ${this.renderTags()}
          ${this.renderExperiments()}
          <h3>Input Properties</h3>
          <milo-property-viewer
            .properties=${build.input?.properties || {}}
            .config=${this.store.userConfig.build.inputProperties}
          ></milo-property-viewer>
          <h3>Output Properties</h3>
          <milo-property-viewer
            .properties=${build.output?.properties || {}}
            .config=${this.store.userConfig.build.outputProperties}
          ></milo-property-viewer>
          ${this.renderBuildPackages()}
        </div>
      </div>
    `;
  }

  static styles = [
    commonStyle,
    colorClasses,
    css`
      #main {
        margin: 10px 16px;
      }
      @media screen and (min-width: 1300px) {
        #main {
          display: grid;
          grid-template-columns: 1fr 20px 40vw;
        }
        .second-column > h3:first-child {
          margin-block: 5px 10px;
        }
      }
      .first-column {
        overflow: hidden;
        grid-column: 1;
      }
      .second-column {
        overflow: hidden;
        grid-column: 3;
      }

      h3 {
        margin-block: 15px 10px;
      }
      h4 {
        margin-block: 10px 10px;
      }

      #summary-html {
        padding: 0 10px;
        clear: both;
        overflow-wrap: break-word;
      }
      #summary-html pre {
        white-space: pre-wrap;
        overflow-wrap: break-word;
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
        padding: 5px;
      }
      .warning {
        background-color: var(--warning-color);
        font-weight: 500;
      }

      #scheduled-cancel {
        background-color: var(--canceled-bg-color);
        font-weight: 500;
        padding: 5px;
      }

      td:nth-child(2) {
        clear: both;
        overflow-wrap: anywhere;
      }

      /*
       * Use normal text color and add extra margin so it's easier to tell where
       * does the failed tests section ends and the steps section starts.
       */
      #failed-tests-section-body {
        margin-bottom: 25px;
      }
      #failed-test-count > a {
        color: var(--default-text-color);
      }

      #failed-test-count {
        margin-top: 10px;
      }

      #loading-spinner {
        color: var(--active-text-color);
      }

      #step-buttons {
        float: right;
      }

      #step-config {
        margin-bottom: 10px;
      }

      #debug-logs-label {
        margin-right: 50px;
      }

      milo-property-viewer {
        min-width: 600px;
        max-width: 1000px;
      }

      mwc-button {
        width: 155px;
      }

      .inline-icon {
        --mdc-icon-size: 1.2em;
        vertical-align: bottom;
        width: 16px;
        height: 16px;
      }

      #build-pkgs-table {
        width: 100%;
        text-align: left;
        table-layout: fixed;
      }
      #build-pkgs-table td {
        padding: 0.1em 1em 0.1em 1em;
      }
      #build-pkgs-table > tbody > tr:nth-child(even) {
        background-color: var(--block-background-color);
      }

      #build-pkgs-table td:first-child {
        width: 20%;
        min-width: 10%;
        max-width: 50%;
      }

      .nav-scrollbar {
        overflow-x: scroll;
        white-space: nowrap;
      }

      .nested-manifest-table {
        text-align: left;
      }
    `,
  ];
}
