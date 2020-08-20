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
import { css, customElement, html } from 'lit-element';
import MarkdownIt from 'markdown-it';
import { observable } from 'mobx';

import '../../components/link';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { getBotLink, getURLForGerritChange, getURLForGitilesCommit, getURLForSwarmingTask } from '../../libs/build_utils';
import { sanitizeHTML } from '../../libs/sanitize_html';
import { displayDuration, displayTimestamp } from '../../libs/time_utils';
import { BuildStatus, Timestamp } from '../../services/buildbucket';

const STATUS_DISPLAY_MAP = new Map([
  [BuildStatus.Scheduled, 'scheduled'],
  [BuildStatus.Started, 'started'],
  [BuildStatus.Success, 'succeeded'],
  [BuildStatus.Failure, 'failed'],
  [BuildStatus.InfraFailure, 'infra failed'],
  [BuildStatus.Canceled, 'canceled'],
]);

const STATUS_CLASS_MAP = new Map([
  [BuildStatus.Scheduled, 'scheduled'],
  [BuildStatus.Started, 'started'],
  [BuildStatus.Success, 'success'],
  [BuildStatus.Failure, 'failure'],
  [BuildStatus.InfraFailure, 'infra-failure'],
  [BuildStatus.Canceled, 'canceled'],
]);

export class OverviewTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'overview';
  }

  private renderStatusTime() {
    const bpd = this.buildState.buildPageData!;

    return html`
      Build
      <span class="status ${STATUS_CLASS_MAP.get(bpd.status)}">
        ${STATUS_DISPLAY_MAP.get(bpd.status) || 'unknown status'}
      </span>
      ${(() => { switch (bpd.status) {
      case BuildStatus.Scheduled:
        return `since ${displayTimestamp(bpd.create_time)}`;
      case BuildStatus.Started:
        return `since ${displayTimestamp(bpd.start_time)}`;
      case BuildStatus.Canceled:
        return `after ${displayDuration(bpd.start_time, bpd.end_time)} by ${bpd.canceled_by}`;
      case BuildStatus.Failure:
      case BuildStatus.InfraFailure:
      case BuildStatus.Success:
        return `after ${displayDuration(bpd.start_time, bpd.end_time)}`;
      default:
        return '';
      }})()}
    `;
  }

  private renderSummary() {
    const bpd = this.buildState.buildPageData!;
    if (bpd.summary_markdown) {
      const md = new MarkdownIt();
      return html`
        <div id="summary-html">
          ${sanitizeHTML(md.render(bpd.summary_markdown))}
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
        return displayDuration(start, end);
      }
      return 'N/A';
    };

    return html`
      <div>
        <h3>Timing</h3>
        <table>
          <tr><td>Create:</td><td>${displayTimestamp(bpd.create_time)}</td></tr>
          <tr><td>Start:</td><td>${displayTimestamp(bpd.start_time)}</td></tr>
          <tr><td>End:</td><td>${displayTimestamp(bpd.end_time)}</td></tr>
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

  protected render() {
    const bpd = this.buildState.buildPageData;
    if (!bpd) {
      return html``;
    }

    return html`
      <div id="status">${this.renderStatusTime()}</div>
      <!-- TODO(crbug/1116824): render action buttons -->
      ${this.renderSummary()}
      ${this.renderInput()}
      ${this.renderInfra()}
      <!-- TODO(crbug/1116824): render failed tests -->
      <!-- TODO(crbug/1116824): render failed steps -->
      ${this.renderTiming()}
      ${this.renderTags()}
      <!-- TODO(crbug/1116824): render properties -->
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

    #summary-html {
      background-color: rgb(245, 245, 245);
      padding: 5px;
    }
  `;
}

customElement('milo-overview-tab')(
  consumeBuildState(
    consumeAppState(OverviewTabElement),
  ),
);
