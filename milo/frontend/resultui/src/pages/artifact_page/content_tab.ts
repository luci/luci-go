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
import * as Diff2Html from 'diff2html';
import { css, customElement, html } from 'lit-element';
import { observable } from 'mobx';

import '../../components/left_panel';
import '../../components/test_entry';
import '../../components/test_filter';
import '../../components/test_nav_tree';
import { consumeContext } from '../../libs/context';
import { sanitizeHTML } from '../../libs/sanitize_html';
import { ArtifactPageState, ArtifactType } from './context';


/**
 * Display the content of the artifact.
 */
export class ArtifactContentTabElement extends MobxLitElement {
  @observable.ref pageState!: ArtifactPageState;

  connectedCallback() {
    super.connectedCallback();
    this.pageState.selectedTabId = 'content';
  }

  protected render() {
    if (this.pageState.artifactType === null) {
      return html``;
    }
    if (this.pageState.artifactType === ArtifactType.TextDiff) {
      return html`
        <link rel="stylesheet" type="text/css" href="https://cdn.jsdelivr.net/npm/diff2html/bundles/css/diff2html.min.css" />
        <div id="content">
          ${sanitizeHTML(Diff2Html.html(this.pageState.content, {drawFileList: false, outputFormat: 'side-by-side'}))}
        </div>
      `;
    }
    return html`Unsupported artifact type`;
  }

  static styles = css`
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

customElement('tr-artifact-content-tab')(
  consumeContext('pageState')(
      ArtifactContentTabElement,
  ),
);
