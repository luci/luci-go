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
import { customElement, html } from 'lit-element';
import { repeat } from 'lit-html/directives/repeat';
import MarkdownIt from 'markdown-it';
import { observable} from 'mobx';

import { sanitizeHTML } from '../../src/libs/sanitize_html';
import { AppState, consumeAppState } from '../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../context/build_state/build_state';
import { router } from '../routes';
import { Build, BuildStatus, Timestamp } from '../services/buildbucket';


export class RelatedBuildsTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'related-builds';
  }

  protected render() {
    return html`
      <h3> Other builds with the same buildset </h3>
      ${this.renderBuildsetInfo()}
      ${this.renderRelatedBuildsTable()}
    `;
  }

  private renderBuildsetInfo() {
    if (this.buildState.buildPageData == null) {
      return html `Loading...`
    }
    return html`
      <ul>
      ${repeat(
        this.buildState.buildPageData!.build_sets,
        (item, _) => html`<li>${item}</li>`
      )}
      </ul>
    `;
  }

  private renderRelatedBuildsTable() {
    if (this.buildState.relatedBuildsData == null) {
      return html ``;
    }
    return html`
      <table>
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
          this.buildState.relatedBuildsData.related_builds,
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
        <td>${build.builder.builder}</td>
        <td>${this.generateBuildLink(build)}</td>
        <td>${BuildStatus[build.status]}</td>
        <td>${this.displayTimestamp(build.create_time)}</td>
        <td>${this.displayDuration(build.create_time, build.start_time)}</td>
        <td>${this.displayDuration(build.start_time, build.end_time)}</td>
        <td>${this.displayMarkdown(build.summary_markdown)}</td>
    `;
  }

  // TODO (crbug.com/1112224): extract this to a library
  private generateBuildLink(build: Build) {
    const href = router.urlForName(
      'build-test-results',
      {
        'project': build.builder.project,
        'bucket': build.builder.bucket,
        'builder': build.builder.builder,
        'build_num_or_id': 'b' + build.id,
      },
    );
    const display = build.number ? build.number : build.id;
    return html`<a href="${href}">${display}</a>`;
  }

  // TODO (crbug.com/1112224): format and extract this to a library
  private displayTimestamp(t: Timestamp) {
    var d = new Date(t.seconds * 1000);
    return html `${d.toString()}`;
  }

  // TODO (crbug.com/1112224): format and extract this to a library
  private displayDuration(beginTime: Timestamp, endTime: Timestamp) {
    return html`${(endTime.seconds - beginTime.seconds) + " secs"}`;
  }

  private displayMarkdown(markdown: string|undefined) {
    if (markdown == undefined) {
      return html ``;
    }
    const md = new MarkdownIt();
    return sanitizeHTML(md.render(markdown));
  }
}

customElement('tr-related-builds-tab')(
  consumeBuildState(
    consumeAppState(RelatedBuildsTabElement),
  )
);
