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

import '@material/mwc-icon';
import { css, customElement, html, LitElement, property } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';

/**
 * Renders an expandable entry.
 */
@customElement('tr-expandable-entry')
export class ExpandableEntry extends LitElement {
  @property() hideContentRuler = false;
  onToggle = (_isExpanded: boolean) => {};

  @property() private _expanded = false;
  get expanded() {
    return this._expanded;
  }
  set expanded(isExpanded) {
    if (isExpanded === this._expanded) {
      return;
    }
    this._expanded = isExpanded;
    this.onToggle(this._expanded);
  }

  protected render() {
    return html`
      <div>
        <div
          id="expandable-header"
          @click=${() => this.expanded = !this.expanded}
        >
          <mwc-icon id="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
          <span id="one-line-content">
            <slot name="header"></slot>
          </span>
        </div>
        <div id="body">
          <div id="content-ruler" style=${styleMap({'display': this.hideContentRuler ? 'none' : ''})}></div>
          <div id="content" style=${styleMap({display: this.expanded ? '' : 'none'})}>
            <slot name="content"></slot>
          </div>
        </div>
      </div>
    `;
  }

  static styles = css`
    :host {
      display: block;
    }

    #expandable-header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: 24px;
      grid-gap: 5px;
      cursor: pointer;
    }
    #expand-toggle {
      grid-row: 1;
      grid-column: 1;
    }
    #one-line-content {
      grid-row: 1;
      grid-column: 2;
      line-height: 24px;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }

    #body {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-gap: 5px;
    }
    #content-ruler {
      grid-row: 1;
      grid-column: 1;
      border-left: 1px solid #DDDDDD;
      width: 0px;
      margin-left: 11.5px;
    }
    #content {
      grid-row: 1;
      grid-column: 2;
      overflow: hidden;
    }
    `;
}
