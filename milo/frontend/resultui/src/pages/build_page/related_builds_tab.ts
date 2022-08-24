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
import { repeat } from 'lit-html/directives/repeat';
import { makeObservable, observable } from 'mobx';

import '../../components/dot_spinner';
import { BuildState, consumeBuildState } from '../../context/build_state';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import { getURLPathForBuild, getURLPathForBuilder } from '../../libs/build_utils';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_DISPLAY_MAP } from '../../libs/constants';
import { consumer } from '../../libs/context';
import { errorHandler, forwardWithoutMsg, reportRenderError } from '../../libs/error_handler';
import { renderMarkdown } from '../../libs/markdown_utils';
import { displayDuration, NUMERIC_TIME_FORMAT } from '../../libs/time_utils';
import { BuildExt } from '../../models/build_ext';
import { consumeStore, StoreInstance } from '../../store';
import commonStyle from '../../styles/common_style.css';

@customElement('milo-related-builds-tab')
@errorHandler(forwardWithoutMsg)
@consumer
export class RelatedBuildsTabElement extends MobxLitElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref
  @consumeBuildState()
  buildState!: BuildState;

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();
    this.store.setSelectedTabId('related-builds');
    trackEvent(GA_CATEGORIES.RELATED_BUILD_TAB, GA_ACTIONS.TAB_VISITED, window.location.href);
  }

  protected render = reportRenderError(this, () => {
    if (this.buildState.relatedBuilds === null) {
      return this.renderLoadingBar();
    }
    if (this.buildState.relatedBuilds.length === 0) {
      return this.renderNoRelatedBuilds();
    }
    return html` ${this.renderBuildsetInfo()} ${this.renderRelatedBuildsTable()} `;
  });

  private renderLoadingBar() {
    return html` <div id="load">Loading <milo-dot-spinner></milo-dot-spinner></div> `;
  }

  private renderNoRelatedBuilds() {
    return html` <div id="no-related-builds">No other builds found with the same buildset.</div> `;
  }

  private renderBuildsetInfo() {
    if (this.buildState.build === null) {
      return html``;
    }
    return html`
      <h3>Other builds with the same buildset</h3>
      <ul>
        ${repeat(this.buildState.build.buildSets, (item, _) => html`<li>${item}</li>`)}
      </ul>
    `;
  }

  private renderRelatedBuildsTable() {
    if (this.buildState.relatedBuilds === null) {
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
        ${repeat(this.buildState.relatedBuilds, (relatedBuild, _) => this.renderRelatedBuildRow(relatedBuild))}
      </table>
    `;
  }

  private renderRelatedBuildRow(build: BuildExt) {
    return html`
      <tr>
        <td>${build.builder.project}</td>
        <td>${build.builder.bucket}</td>
        <td><a href=${getURLPathForBuilder(build.builder)}>${build.builder.builder}</a></td>
        <td>${this.renderBuildLink(build)}</td>
        <td class="status ${BUILD_STATUS_CLASS_MAP[build.status]}">
          ${BUILD_STATUS_DISPLAY_MAP[build.status] || 'unknown'}
        </td>
        <td>${build.createTime.toFormat(NUMERIC_TIME_FORMAT)}</td>
        <td>${displayDuration(build.pendingDuration) || 'N/A'}</td>
        <td>${(build.executionDuration && displayDuration(build.executionDuration)) || 'N/A'}</td>
        <td>${renderMarkdown(build.summaryMarkdown || '')}</td>
      </tr>
    `;
  }

  private renderBuildLink(build: BuildExt) {
    const display = build.number ? build.number : build.id;
    return html`<a href=${getURLPathForBuild(build)}>${display}</a>`;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: block;
        padding-left: 10px;
      }

      tr:nth-child(even) {
        background-color: var(--block-background-color);
      }
      #related-builds-table {
        vertical-align: middle;
        text-align: center;
      }
      #related-builds-table td {
        padding: 0.1em 1em 0.1em 1em;
      }
      .status.success {
        background-color: #8d4;
        border-color: #4f8530;
      }
      .status.failure {
        background-color: #e88;
        border-color: #a77272;
        border-style: solid;
      }
      .status.infra-failure {
        background-color: #c6c;
        border-color: #aca0b3;
        color: #ffffff;
      }
      .status.started {
        background-color: #fd3;
        border-color: #c5c56d;
      }
      .status.canceled {
        background-color: #8ef;
        border-color: #00d8fc;
      }
      #load {
        padding: 10px;
        color: var(--active-text-color);
      }
      #no-related-builds {
        padding: 10px;
      }
    `,
  ];
}
