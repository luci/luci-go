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
import { css,customElement, html } from 'lit-element';
import { repeat } from 'lit-html/directives/repeat';
import { observable } from 'mobx';

import '../../components/dot_spinner';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { getDisplayNameForStatus, getURLForBuild, getURLForBuilder } from '../../libs/build_utils';
import { displayTimeDiffOpt, displayTimestamp } from '../../libs/time_utils';
import { renderMarkdown } from '../../libs/utils';
import { Build, BuildStatus } from '../../services/buildbucket';

export class RelatedBuildsTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'related-builds';
  }

  protected render() {
    return html`
      <div id = "main">
        <h3> Other builds with the same buildset </h3>
        ${this.renderBuildsetInfo()}
        ${this.renderRelatedBuildsTable()}
        ${this.renderLoadingBar()}
      </div>
    `;
  }

  private renderLoadingBar() {
    if (this.buildState.buildPageData == null || this.buildState.relatedBuildsData == null) {
      return html`
        <div id="load">
          Loading <milo-dot-spinner></milo-dot-spinner>
        </div>
      `;
    }
    return html ``;
  }

  private renderBuildsetInfo() {
    if (this.buildState.buildPageData == null) {
      return html``;
    }
    return html`
      <ul>
      ${repeat(
        this.buildState.buildPageData.buildSets,
        (item, _) => html`<li>${item}</li>`,
      )}
      </ul>
    `;
  }

  private renderRelatedBuildsTable() {
    if (this.buildState.relatedBuildsData == null) {
      return html``;
    }
    return html`
      <table id="related-builds-table">
        <tr>
          <th>Project</th>
          <th>Bucket</th>
          <th>Builder</th>
          <th>Build</th>
          <th>Status</th>
          <th>Create Time</th>
          <th>Pending</th>
          <th>Duration</th>
          <th>Summary</th>
        </tr>
        ${repeat(
          this.buildState.relatedBuildsData.relatedBuilds,
          (relatedBuild, _) => this.renderRelatedBuildRow(relatedBuild),
        )}
      </table>
    `;
  }

  private renderRelatedBuildRow(build: Build) {
    return html `
      <tr>
        <td>${build.builder.project}</td>
        <td>${build.builder.bucket}</td>
        <td><a href=${getURLForBuilder(build.builder)}>${build.builder.builder}</a></td>
        <td>${this.renderBuildLink(build)}</td>
        <td class="status ${BuildStatus[build.status]}">${getDisplayNameForStatus(build.status)}</td>
        <td>${displayTimestamp(build.createTime)}</td>
        <td>${displayTimeDiffOpt(build.createTime, build.startTime) || 'N/A'}</td>
        <td>${displayTimeDiffOpt(build.startTime, build.endTime) || 'N/A'}</td>
        <td>${renderMarkdown(build.summaryMarkdown || '')}</td>
      </tr>
    `;
  }

  private renderBuildLink(build: Build) {
    const display = build.number ? build.number : build.id;
    return html`<a href=${getURLForBuild(build)}>${display}</a>`;
  }

  static styles = css`
    tr:nth-child(even) {
      background-color: var(--block-background-color);
    }
    #related-builds-table {
      vertical-align: middle;
      text-align: center;
    }
    #related-builds-table td{
      padding: 0.1em 1em 0.1em 1em;
    }
    .status.Success {
      background-color: #8d4;
      border-color: #4f8530;
    }
    .status.Failure {
      background-color: #e88;
      border-color: #a77272;
      border-style: solid;
    }
    .status.InfraFailure {
      background-color: #c6c;
      border-color: #aca0b3;
      color: #ffffff;
    }
    .status.Started {
      background-color: #fd3;
      border-color: #c5c56d;
    }
    .status.Canceled {
      background-color: #8ef;
      border-color: #00d8fc;
    }
    #load {
      color: var(--active-text-color);
    }
    #main {
      padding-left: 10px;
    }
  `;
}

customElement('milo-related-builds-tab')(
  consumeBuildState(
    consumeAppState(RelatedBuildsTabElement),
  ),
);
