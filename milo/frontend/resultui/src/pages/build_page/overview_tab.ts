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
import { STATUS_CLASS_MAP, STATUS_DISPLAY_MAP } from '.';

import '../../components/ace_editor';
import '../../components/link';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { getBotLink, getURLForGerritChange, getURLForGitilesCommit, getURLForSwarmingTask } from '../../libs/build_utils';
import { displayTimeDiff, displayTimestamp } from '../../libs/time_utils';
import { renderMarkdown } from '../../libs/utils';
import { router } from '../../routes';
import { BuildStatus, Timestamp } from '../../services/buildbucket';

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
        <i class="status ${STATUS_CLASS_MAP[bpd.status]}">
          ${STATUS_DISPLAY_MAP[bpd.status] || 'unknown status'}
        </i>
        ${(() => { switch (bpd.status) {
        case BuildStatus.Scheduled:
          return `since ${displayTimestamp(bpd.create_time)}`;
        case BuildStatus.Started:
          return `since ${displayTimestamp(bpd.start_time!)}`;
        case BuildStatus.Canceled:
          return `after ${displayTimeDiff(bpd.create_time, bpd.end_time!)} by ${bpd.canceled_by}`;
        case BuildStatus.Failure:
        case BuildStatus.InfraFailure:
        case BuildStatus.Success:
          return `after ${displayTimeDiff(bpd.start_time || bpd.create_time, bpd.end_time!)}`;
        default:
          return '';
        }})()}
        ${bpd.end_time ?
          html`<mwc-button dense unelevated @click=${() => this.showRetryDialog = true}>Retry</mwc-button>` :
          html`<mwc-button dense unelevated @click=${() => this.showCancelDialog = true}>Cancel</mwc-button>`}
      </div>
    `;
  }

  private async cancelBuild(reason: string) {
    await this.appState.buildsService!.cancelBuild({
      id: this.buildState.buildPageData!.id,
      summary_markdown: reason,
    });
    this.buildState.refresh();
  }

  private async retryBuild() {
    const build = await this.appState.buildsService!.scheduleBuild({
      template_build_id: this.buildState.buildPageData!.id,
    });
    Router.go({
      pathname: router.urlForName('build', {
        project: build.builder.project,
        bucket: build.builder.bucket,
        builder: build.builder.builder,
        build_num_or_id: build.number,
      }),
    });
  }

  private renderSummary() {
    const bpd = this.buildState.buildPageData!;
    if (bpd.summary_markdown) {
      return html`
        <div id="summary-html">
          ${renderMarkdown(bpd.summary_markdown)}
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
          ${input.gitiles_commit ? html`
            <tr>
              <td>Revision:</td>
              <td>
                <a href=${getURLForGitilesCommit(input.gitiles_commit)} target="_blank">${input.gitiles_commit.id}</a>
                ${input.gitiles_commit.position ? `CP #${input.gitiles_commit.position}` : ''}
              </td>
            </tr>
          ` : ''}
          ${(input.gerrit_changes || []).map((gc) => html`
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
          <tr><td>Buildbucket ID:</td><td><milo-link .link=${bpd.buildbucket_link} target="_blank"></td></tr>
          ${bpd.infra?.swarming ? html`
          <tr>
            <td>Swarming Task:</td>
            <td>${bpd.infra.swarming.task_id ? html`<a href=${getURLForSwarmingTask(bpd.infra.swarming)}>${bpd.infra.swarming.task_id}</a>`: 'N/A'}</td>
          </tr>
          <tr>
            <td>Bot:</td>
            <td>${botLink ? html`<milo-link .link=${botLink} target="_blank"></milo-link>` : 'N/A'}</td>
          </tr>
          ` : ''}
          <tr><td>Recipe:</td><td><milo-link .link=${bpd.recipe_link} target="_blank"></milo-link></td></tr>
        </table>
      </div>
    `;
  }

  private renderTiming() {
    const bpd = this.buildState.buildPageData!;
    const displayDurationOpt = (start?: Timestamp, end?: Timestamp) => {
      if (start && end) {
        return displayTimeDiff(start, end);
      }
      return 'N/A';
    };

    return html`
      <div>
        <h3>Timing</h3>
        <table>
          <tr><td>Create:</td><td>${displayTimestamp(bpd.create_time)}</td></tr>
          <tr><td>Start:</td><td>${bpd.start_time ? displayTimestamp(bpd.start_time) : 'N/A'}</td></tr>
          <tr><td>End:</td><td>${bpd.end_time ? displayTimestamp(bpd.end_time) : 'N/A'}</td></tr>
          <tr><td>Pending:</td><td>${displayDurationOpt(bpd.create_time, bpd.start_time)}</td></tr>
          <tr><td>Execution:</td><td>${displayDurationOpt(bpd.start_time, bpd.end_time)}</td></tr>
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
      fontSize: 14,
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
      ${this.renderStatusTime()}
      <!-- TODO(crbug/1116824): render action buttons -->
      ${this.renderSummary()}
      ${this.renderInput()}
      ${this.renderInfra()}
      <!-- TODO(crbug/1116824): render failed tests -->
      <!-- TODO(crbug/1116824): render failed steps -->
      ${this.renderTiming()}
      ${this.renderTags()}
      ${this.renderProperties('Input Properties', bpd.input.properties)}
      ${this.renderProperties('Output Properties', bpd.input.properties)}
    `;
  }

  static styles = css`
    :host > * {
      margin: 5px 24px;
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

    :host > mwc-dialog {
      margin: 0 0;
    }
    #cancel-reason {
      width: 500px;
      height: 200px;
    }
    mwc-button {
      --mdc-theme-primary: rgb(0, 123, 255);
      transform: scale(0.8);
      vertical-align: middle;
    }

    #summary-html {
      background-color: rgb(245, 245, 245);
      padding: 5px;
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
