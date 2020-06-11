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
import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import * as Diff2Html from 'diff2html';
import { css, customElement, html } from 'lit-element';
import { computed, observable } from 'mobx';
import { fromPromise } from 'mobx-utils';

import '../components/status_bar';
import { AppState, consumeAppState } from '../context/app_state_provider';
import { sanitizeHTML } from '../libs/sanitize_html';
import { router } from '../routes';

enum ArtifactType {
  /**
   * A text diff artifact.
   */
  TextDiff,
  /**
   * Unsupported artifact type.
   */
  Unsupported,
}

/**
 * Renders an Artifact.
 */
// TODO(weiweilin): improve error handling.
export class ArtifactPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  @observable.ref private artifactName!: string;

  @computed private get invocationId() { return this.parsedArtifactName.invocationId; }
  @computed private get testId() { return this.parsedArtifactName.testId; }
  @computed private get resultId() { return this.parsedArtifactName.resultId; }
  @computed private get artifactId() { return this.parsedArtifactName.artifactId; }
  @computed private get parsedArtifactName() {
    const groups = this.artifactName
      .match(/invocations\/(?<invocationId>.*?)\/(tests\/(?<testId>.*?)\/results\/(?<resultId>.*?)\/)?artifacts\/(?<artifactId>.*)/)!
      .groups!;
    return {
      invocationId: decodeURIComponent(groups['invocationId']),
      testId: groups['testId'] ? decodeURIComponent(groups['testId']) : null,
      resultId: groups['resultId'] ? decodeURIComponent(groups['resultId']) : null,
      artifactId: decodeURIComponent(groups['artifactId']),
    };
  }

  @computed({keepAlive: true})
  private get artifactRes() {
    if (!this.appState.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getArtifact({name: this.artifactName}));
  }
  @computed private get artifact() {
    return this.artifactRes.state === 'fulfilled' ? this.artifactRes.value : null;
  }

  @computed private get artifactType() {
    if (!this.artifact) {
      return null;
    }
    if (this.artifact.contentType === 'text/plain' && this.artifact.artifactId.match(/_diff$/)) {
      return ArtifactType.TextDiff;
    }
    return ArtifactType.Unsupported;
  }

  @computed({keepAlive: true})
  private get contentRes() {
    if (!this.appState.resultDb || !this.artifact) {
      return fromPromise(Promise.race([]));
    }
    // TODO(weiweilin): handle refresh.
    return fromPromise(fetch(this.artifact.fetchUrl!).then((res) => res.text()));
  }
  @computed private get content() {
    return this.contentRes.state === 'fulfilled' ? this.contentRes.value : '';
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const artifactName = location.params['artifact_name'];
    if (typeof artifactName !== 'string') {
      return cmd.redirect('/not-found');
    }
    this.artifactName = artifactName;
    return;
  }

  private renderContent() {
    if (this.artifactType === null) {
      return html``;
    }
    if (this.artifactType === ArtifactType.TextDiff) {
      return html`
        <link rel="stylesheet" type="text/css" href="https://cdn.jsdelivr.net/npm/diff2html/bundles/css/diff2html.min.css" />
        ${sanitizeHTML(Diff2Html.html(this.content, {drawFileList: false, outputFormat: 'side-by-side'}))}
      `;
    }
    return html`Unsupported artifact type`;
  }

  protected render() {
    return html`
      <div id="artifact-header">
        <table>
          <tr>
            <td class="id-component-label">Invocation</td>
            <td><a href=${router.urlForName('invocation', {'invocation_id': this.invocationId})}>${this.invocationId}</a></td>
          </tr>
          <!-- TODO(weiweilin): add view test link -->
          <tr>
            <td class="id-component-label">Test</td>
            <td>${this.testId}</td>
          </tr>
          <!-- TODO(weiweilin): add view result link -->
          <tr>
            <td class="id-component-label">Result</td>
            <td>${this.resultId}</td>
          </tr>
          <tr>
            <td class="id-component-label">Artifact</td>
            <td>${this.artifactId}</td>
          </tr>
        </table>
      </div>
      <tr-status-bar
        .components=${[{color: '#007bff', weight: 1}]}
        .loading=${this.artifactRes.state === 'pending'}
      ></tr-status-bar>
      <div id="details">
        ${this.artifact?.fetchUrl ? html`<a href=${this.artifact?.fetchUrl}>View Raw Content</a>` : ''}
      </div>
      <div id="content">${this.renderContent()}</div>
    `;
  }

  static styles = css`
    #artifact-header {
      background-color: rgb(248, 249, 250);
      padding: 6px 16px;
      font-family: "Google Sans", "Helvetica Neue", sans-serif;
      font-size: 14px;
    }
    .id-component-label {
      color: rgb(95, 99, 104);
    }

    #details {
      margin: 20px;
    }
    #content {
      margin: 20px;
    }

    .d2h-code-linenumber {
      cursor: default;
    }
    .d2h-moved-tag {
      display: none;
    }
  `;
}

customElement('tr-artifact-page')(
  consumeAppState(
    ArtifactPageElement,
  ),
);
