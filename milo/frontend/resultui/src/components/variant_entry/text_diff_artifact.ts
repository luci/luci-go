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
import '@material/mwc-icon';
import * as Diff2Html from 'diff2html';
import { css, customElement, html, property } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import { sanitizeHTML } from '../../libs/sanitize_html';
import { router } from '../../routes';
import { Artifact } from '../../services/resultdb';

/**
 * Renders a text diff artifact.
 */
@customElement('tr-text-diff-artifact')
export class TextDiffArtifactElement extends MobxLitElement {
  @property() artifact!: Artifact;

  @observable.ref private expanded = true;

  @computed
  private get contentRes(): IPromiseBasedObservable<string> {
    // TODO(weiweilin): handle refresh.
    return fromPromise(fetch(this.artifact.fetchUrl!).then((res) => res.text()));
  }

  @computed
  private get content() {
    return this.contentRes.state === 'fulfilled' ? this.contentRes.value : '';
  }

  protected render() {
    return html`
      <link rel="stylesheet" type="text/css" href="https://cdn.jsdelivr.net/npm/diff2html/bundles/css/diff2html.min.css" />
      <div
        class="expandable-header"
        @click=${() => this.expanded = !this.expanded}
      >
        <mwc-icon class="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <span class="one-line-content">
          Unexpected text output from
          <a href=${router.urlForName('text-diff-artifact', {'artifact_name': this.artifact.name})} target="_blank">${this.artifact.artifactId}</a>
          (<a href=${this.artifact.fetchUrl} target="_blank">view raw</a>)
        </span>
      </div>
      <div id="container" style=${styleMap({display: this.expanded ? '' : 'none'})}>
        ${sanitizeHTML(Diff2Html.html(this.content, {drawFileList: false}))}
      </div>
    `;
  }

  static styles = css`
    .expandable-header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: 24px;
      grid-gap: 5px;
      cursor: pointer;
    }
    .expandable-header .expand-toggle {
      grid-row: 1;
      grid-column: 1;
    }
    .expandable-header .one-line-content {
      grid-row: 1;
      grid-column: 2;
      line-height: 24px;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }
    #entry-header .one-line-content {
      font-size: 14px;
      letter-spacing: 0.1px;
      font-weight: 500;
    }

    #container {
      padding: 0 10px;
      margin: 5px;
      position: relative;
      overflow-y: auto;
      max-height: 500px;
    }
    .d2h-code-linenumber {
      cursor: default;
    }
    .d2h-moved-tag {
      display: none;
    }
  `;
}
