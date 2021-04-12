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
import { fromPromise, FULFILLED, PENDING } from 'mobx-utils';

import '../../components/status_bar';
import './artifact_page_layout';
import { AppState, consumeAppState } from '../../context/app_state';
import { sanitizeHTML } from '../../libs/sanitize_html';
import { NOT_FOUND_URL } from '../../routes';
import { parseArtifactName } from '../../services/resultdb';

/**
 * Renders a text diff artifact.
 */
// TODO(weiweilin): improve error handling.
@customElement('milo-text-diff-artifact-page')
@consumeAppState
export class TextDiffArtifactPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  @observable.ref private artifactName!: string;

  @computed private get artifactIdent() {
    return parseArtifactName(this.artifactName);
  }

  @computed
  private get artifact$() {
    if (!this.appState.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getArtifact({ name: this.artifactName }));
  }
  @computed private get artifact() {
    return this.artifact$.state === FULFILLED ? this.artifact$.value : null;
  }

  @computed
  private get content$() {
    if (!this.appState.resultDb || !this.artifact) {
      return fromPromise(Promise.race([]));
    }
    // TODO(weiweilin): handle refresh.
    return fromPromise(fetch(this.artifact.fetchUrl).then((res) => res.text()));
  }
  @computed private get content() {
    return this.content$.state === FULFILLED ? this.content$.value : null;
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const artifactName = location.params['artifact_name'];
    if (typeof artifactName !== 'string') {
      return cmd.redirect(NOT_FOUND_URL);
    }
    this.artifactName = artifactName;
    return;
  }

  protected render() {
    return html`
      <milo-artifact-page-layout .ident=${this.artifactIdent} .isLoading=${this.artifact$.state === PENDING}>
        <div id="details">
          ${this.artifact?.fetchUrl ? html`<a href=${this.artifact?.fetchUrl}>View Raw Content</a>` : ''}
        </div>
        <div id="content">
          <link
            rel="stylesheet"
            type="text/css"
            href="https://cdn.jsdelivr.net/npm/diff2html/bundles/css/diff2html.min.css"
          />
          ${sanitizeHTML(Diff2Html.html(this.content || '', { drawFileList: false, outputFormat: 'side-by-side' }))}
        </div>
      </milo-artifact-page-layout>
    `;
  }

  static styles = css`
    #details {
      margin: 20px;
    }
    #content {
      position: relative;
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
