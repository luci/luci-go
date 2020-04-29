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

import { css, customElement, LitElement, property } from 'lit-element';
import { html } from 'lit-html';
import { classMap } from 'lit-html/directives/class-map';

export interface TabDef {
  id: string;
  label: string;
  href?: string;
}


/**
 * A tab bar element. When a new tab is selected, it can notify the parent
 * element via callback, or navigate to the link provided in the tab config.
 */
@customElement('tr-tab-bar')
export class TabBarElement extends LitElement {
  @property() tabs: TabDef[] = [];
  @property() selectedTabId = '';

  onSelectedTabChanged: (selectedTabId: string) => void = () => {};

  protected render() {
    return html`
      <div id="tab-bar">
        ${this.tabs.map((tab) => html`
          <a
            class=${classMap({'tab': true, 'selected': this.selectedTabId === tab.id})}
            @click=${() => {
              if (this.selectedTabId === tab.id) {
                return;
              }
              this.selectedTabId = tab.id;
              this.onSelectedTabChanged(this.selectedTabId);
            }}
            href=${tab.href || ''}
          >${tab.label}</a>
        `)}
      </div>
    `;
  }

  static styles = css`
    :host {
      margin: 10px;
      margin-bottom: 5px;
    }

    #tab-bar {
      height: 25px;
      border-bottom: 1px solid #EBEBEB;
    }
    .tab {
      color: #212121;
      padding: 7px 14px 7px 14px;
      font-size: 110%;
      text-align: center;
      text-decoration: none;
      cursor: pointer;
    }
    .tab.selected {
      font-weight: bold;
      border-bottom: 3px solid #dd4b39;
      padding-bottom: 4px;
    }
  `;
}
