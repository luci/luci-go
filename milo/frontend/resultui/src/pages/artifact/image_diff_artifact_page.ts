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
import { PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { computed, observable } from 'mobx';
import { fromPromise } from 'mobx-utils';

import '../../components/image_diff_viewer';
import '../../components/status_bar';
import { AppState, consumeAppState } from '../../context/app_state_provider';
import { router } from '../../routes';
import { constructArtifactName, parseArtifactName } from '../../services/resultdb';

/**
 * Renders an image diff artifact set, including expected image, actual image
 * and image diff.
 */
// TODO(weiweilin): improve error handling.
export class ImageDiffArtifactPage extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref diffArtifactName!: string;

  @computed private get expectedArtifactName() { return constructArtifactName({...this.parsedDiffArtifactName, artifactId: this.expectedArtifactId}); }
  @computed private get actualArtifactName() { return constructArtifactName({...this.parsedDiffArtifactName, artifactId: this.actualArtifactId}); }

  @computed private get diffArtifactId() { return this.parsedDiffArtifactName.artifactId; }
  @observable.ref private expectedArtifactId!: string;
  @observable.ref private actualArtifactId!: string;

  @computed private get parsedDiffArtifactName() { return parseArtifactName(this.diffArtifactName); }

  @computed({keepAlive: true})
  private get diffArtifactRes() {
    if (!this.appState.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getArtifact({name: this.diffArtifactName}));
  }
  @computed private get diffArtifact() {
    return this.diffArtifactRes.state === 'fulfilled' ? this.diffArtifactRes.value : null;
  }

  @computed({keepAlive: true})
  private get expectedArtifactRes() {
    if (!this.appState.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getArtifact({name: this.expectedArtifactName}));
  }
  @computed private get expectedArtifact() {
    return this.expectedArtifactRes.state === 'fulfilled' ? this.expectedArtifactRes.value : null;
  }

  @computed({keepAlive: true})
  private get actualArtifactRes() {
    if (!this.appState.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getArtifact({name: this.actualArtifactName}));
  }
  @computed private get actualArtifact() {
    return this.actualArtifactRes.state === 'fulfilled' ? this.actualArtifactRes.value : null;
  }

  @computed get isLoading() {
    return this.expectedArtifactRes.state === 'pending' || this.actualArtifactRes.state === 'pending' || this.diffArtifactRes.state === 'pending';
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const diffArtifactName = location.params['artifact_name'];
    const search = new URLSearchParams(location.search);
    const expectedArtifactId = search.get('expected_artifact_id');
    const actualArtifactId = search.get('actual_artifact_id');
    if (typeof diffArtifactName !== 'string' || !expectedArtifactId || !actualArtifactId) {
      return cmd.redirect('/not-found');
    }

    this.expectedArtifactId = expectedArtifactId;
    this.actualArtifactId = actualArtifactId;
    this.diffArtifactName = diffArtifactName;
    return;
  }


  protected render() {
    return html`
      <div id="artifact-header">
        <table>
          <tr>
            <td class="id-component-label">Invocation</td>
            <td><a href=${router.urlForName('invocation', {'invocation_id': this.parsedDiffArtifactName.invocationId})}>${this.parsedDiffArtifactName.invocationId}</a></td>
          </tr>
          ${this.parsedDiffArtifactName.testId && html`
          <!-- TODO(weiweilin): add view test link -->
          <tr>
            <td class="id-component-label">Test</td>
            <td>${this.parsedDiffArtifactName.testId}</td>
          </tr>
          `}
          ${this.parsedDiffArtifactName.resultId && html`
          <!-- TODO(weiweilin): add view result link -->
          <tr>
            <td class="id-component-label">Result</td>
            <td>${this.parsedDiffArtifactName.resultId}</td>
          </tr>
          `}
          <tr>
            <td class="id-component-label">Artifacts</td>
            <td>${this.expectedArtifactId}, ${this.actualArtifactId}, ${this.diffArtifactId}</td>
          </tr>
        </table>
      </div>
      <tr-status-bar
        .components=${[{color: '#007bff', weight: 1}]}
        .loading=${this.isLoading}
      ></tr-status-bar>
      ${this.isLoading ? '' : html`
        <tr-image-diff-viewer
          .expected=${this.expectedArtifact}
          .actual=${this.actualArtifact}
          .diff=${this.diffArtifact}
        >
      `}
      </tr-image-diff-viewer>
    `;
  }

  static styles = css`
    :host {
      display: block;
    }

    #artifact-header {
      background-color: rgb(248, 249, 250);
      padding: 6px 16px;
      font-family: "Google Sans", "Helvetica Neue", sans-serif;
      font-size: 14px;
    }
    .id-component-label {
      color: rgb(95, 99, 104);
    }
  `;
}

customElement('tr-image-diff-artifact-page')(
  consumeAppState(
    ImageDiffArtifactPage,
  ),
);
