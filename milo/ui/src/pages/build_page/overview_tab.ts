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
import '@material/mwc-textarea';
import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement, html, TemplateResult } from 'lit-element';
import { unsafeHTML } from 'lit-html/directives/unsafe-html';
import { action, computed, makeObservable, observable } from 'mobx';

import '../../components/build_tag_row';
import '../../components/expandable_entry';
import '../../components/link';
import '../../components/log';
import '../../components/property_viewer';
import '../../components/relative_timestamp';
import '../../components/timestamp';
import '../test_results_tab/test_variants_table/test_variant_entry';
import './build_packages_info';
import './cancel_build_dialog';
import './retry_build_dialog';
import './steps_tab/step_display_config';
import './steps_tab/step_list';
import { consumeInvocationState, InvocationState } from '../../context/invocation_state';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import {
  getBotLink,
  getBuildbucketLink,
  getURLForGerritChange,
  getURLForGitilesCommit,
  getURLForSwarmingTask,
} from '../../libs/build_utils';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_DISPLAY_MAP } from '../../libs/constants';
import { consumer } from '../../libs/context';
import { errorHandler, forwardWithoutMsg, reportRenderError } from '../../libs/error_handler';
import { renderMarkdown } from '../../libs/markdown_utils';
import { displayDuration } from '../../libs/time_utils';
import { unwrapOrElse } from '../../libs/utils';
import { router } from '../../routes';
import { ADD_BUILD_PERM, BuildStatus, CANCEL_BUILD_PERM, GitilesCommit } from '../../services/buildbucket';
import { createTVPropGetter, getPropKeyLabel } from '../../services/resultdb';
import { consumeStore, StoreInstance } from '../../store';
import colorClasses from '../../styles/color_classes.css';
import commonStyle from '../../styles/common_style.css';

const MAX_DISPLAYED_UNEXPECTED_TESTS = 10;

const enum Dialog {
  None,
  CancelBuild,
  RetryBuild,
}

@customElement('milo-overview-tab')
@errorHandler(forwardWithoutMsg)
@consumer
export class OverviewTabElement extends MobxLitElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref
  @consumeInvocationState()
  invocationState!: InvocationState;

  @computed private get build() {
    return this.store.buildPage.build;
  }

  @observable.ref private activeDialog = Dialog.None;
  @action private setActiveDialog(dialog: Dialog) {
    this.activeDialog = dialog;
  }

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
    const build = this.store.buildPage.build!;

    const canRetry = this.store.buildPage.permittedActions[ADD_BUILD_PERM] ?? false;

    if (build.endTime) {
      return html`
        <h3>Actions</h3>
        <div title=${canRetry ? '' : 'You have no permission to retry this build.'}>
          <mwc-button dense unelevated @click=${() => this.setActiveDialog(Dialog.RetryBuild)} ?disabled=${!canRetry}>
            Retry Build
          </mwc-button>
        </div>
      `;
    }

    const canCancel = build.cancelTime === null && this.store.buildPage.permittedActions[CANCEL_BUILD_PERM];
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
        <mwc-button dense unelevated @click=${() => this.setActiveDialog(Dialog.CancelBuild)} ?disabled=${!canCancel}>
          Cancel Build
        </mwc-button>
      </div>
    `;
  }

  private renderCanaryWarning() {
    if (!this.store.buildPage.build?.isCanary) {
      return html``;
    }
    if ([BuildStatus.Failure, BuildStatus.InfraFailure].indexOf(this.store.buildPage.build!.data.status) === -1) {
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
    if (!this.build?.cancelTime || this.build?.endTime) {
      return html``;
    }

    // TODO(weiweilin): refresh the build automatically once we reached
    // scheduledCancelTime.
    const scheduledCancelTime = this.build.cancelTime
      // We have gracePeriod since Feb 2021. It's safe to use ! since all new
      // (not-terminated) builds should have gracePeriod defined.
      .plus(this.build.gracePeriod!)
      // Add min_update_interval (currently always 30s).
      // TODO(crbug/1299302): read min_update_interval from buildbucket.
      .plus({ seconds: 30 });

    return html`
      <div id="scheduled-cancel">
        This build was scheduled to be canceled by ${this.build.data.canceledBy || 'unknown'}
        <milo-relative-timestamp .timestamp=${scheduledCancelTime}></milo-relative-timestamp>.
      </div>
    `;
  }

  private renderSummary() {
    const build = this.build!.data!;
    if (!build.summaryMarkdown) {
      return html`
        <div id="summary-html" class="${BUILD_STATUS_CLASS_MAP[build.status]}-bg">
          <div id="status">Build ${BUILD_STATUS_DISPLAY_MAP[build.status] || 'status unknown'}</div>
        </div>
      `;
    }

    return html`
      <div id="summary-html" class="${BUILD_STATUS_CLASS_MAP[build.status]}-bg">
        ${unsafeHTML(renderMarkdown(build.summaryMarkdown))}
      </div>
    `;
  }

  private renderBuilderDescription() {
    const descriptionHtml = unwrapOrElse(
      () => this.store.buildPage.builder?.config.descriptionHtml,
      (err) => {
        // The builder config might've been deleted from buildbucket or the
        // builder is a dynamic builder.
        console.warn('failed to get builder description', err);
        return undefined;
      }
    );
    if (!descriptionHtml) {
      return html``;
    }
    return html`
      <h3>Builder Info</h3>
      <div id="builder-description">${unsafeHTML(descriptionHtml)}</div>
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
    const input = this.build?.data.input;
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
    const output = this.build?.data.output;
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

  private renderInfra() {
    const build = this.store.buildPage.build!.data;
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
                    ${!build.ancestorIds?.length
                      ? 'no ancestor builds'
                      : build.ancestorIds.map((id) => html`<a href="/b/${id}">${id}</a> `)}
                  </td>
                </tr>
              `
            : ''
        }
        <tr>
          <td>Recipe:</td>
          <td><milo-link .link=${this.build!.recipeLink} target="_blank"></milo-link></td>
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
      ...this.store.buildPage.builderIdParam!,
      build_num_or_id: this.store.buildPage.buildNumOrIdParam!,
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
            .historyUrl=${this.invocationState.getHistoryUrl(testVariant.testId, testVariant.variantHash)}
          ></milo-test-variant-entry>
        `);
      }
    }

    return htmlTemplates;
  }

  private renderSteps() {
    const stepsUrl = router.urlForName('build-steps', {
      ...this.store.buildPage.builderIdParam,
      build_num_or_id: this.store.buildPage.buildNumOrIdParam!,
    });
    return html`
      <div>
        <h3>Steps & Logs (<a href=${stepsUrl}>View in Steps Tab</a>)</h3>
        <milo-bp-step-display-config></milo-bp-step-display-config>
        <milo-bp-step-list></milo-bp-step-list>
      </div>
    `;
  }

  private renderTiming() {
    const build = this.store.buildPage.build!;

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
    const tags = this.build?.data.tags;
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
    const experiments = this.build?.data.input?.experiments;
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
    const logs = this.build?.data.output?.logs;
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

  protected render = reportRenderError(this, () => {
    const build = this.store.buildPage.build?.data;
    if (!build) {
      return html``;
    }

    return html`
      <milo-bp-retry-build-dialog
        ?open=${this.activeDialog === Dialog.RetryBuild}
        @close=${() => this.setActiveDialog(Dialog.None)}
      ></milo-bp-retry-build-dialog>
      <milo-bp-cancel-build-dialog
        ?open=${this.activeDialog === Dialog.CancelBuild}
        @close=${() => this.setActiveDialog(Dialog.None)}
      ></milo-bp-cancel-build-dialog>
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
          <milo-bp-build-packages-info .build=${build}></milo-bp-build-packages-info>
        </div>
      </div>
    `;
  });

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
    `,
  ];
}
