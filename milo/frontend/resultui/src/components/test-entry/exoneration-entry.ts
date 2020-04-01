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
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { observable } from 'mobx';

import { sanitizeHTML } from '../../libs/sanitize_html';
import { TestExoneration } from '../../services/resultdb';


/**
 * Renders an expandable entry of the given test exoneration.
 */
@customElement('tr-exoneration-entry')
export class ExonerationEntryElement extends MobxLitElement {
  @observable.ref id = '';
  @observable.ref testExoneration?: TestExoneration;
  @observable.ref expanded = false;

  @observable.ref private explanationExpanded = true;

  private renderExplanationHTML() {
    if (!this.testExoneration!.explanationHTML) {
      return html``;
    }

    return html`
      <div
        class="expandable-header"
        @click=${() => this.explanationExpanded = !this.explanationExpanded}
      >
        <mwc-icon class="expand-toggle">${this.explanationExpanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <div class="one-line-content">Explanation:</div>
      </div>
      <div id="explanation-html" style=${styleMap({display: this.explanationExpanded ? '' : 'none'})}>
        ${sanitizeHTML(this.testExoneration!.explanationHTML)}
      </div>
    `;
  }

  protected render() {
    return html`
      <div>
        <div
          id="entry-header"
          class="expandable-header"
          @click=${() => this.expanded = !this.expanded}
        >
          <mwc-icon class="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
          <span class="one-line-content">
            #${this.id}
            <span style="color: rgb(210, 63, 49);">exonerated</span>
          </span>
        </div>
        <div id="body">
          <div id="content-ruler"></div>
          <div id="content" style=${styleMap({display: this.expanded ? '' : 'none'})}>
            ${this.renderExplanationHTML()}
          </div>
        </div>
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
      font-size: 16px;
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

    #body {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-gap: 5px;
    }
    #content-ruler {
      border-left: 1px solid #DDDDDD;
      width: 0px;
      margin-left: 11.5px;
    }
    #explanation-html {
      background-color: rgb(245, 245, 245);
      padding: 5px;
    }
  `;
}
