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
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { classMap } from 'lit/directives/class-map.js';
import { makeObservable, observable } from 'mobx';

export interface TabDef {
  id: string;
  label: string;
  href?: string;
  slotName?: string;
}

/**
 * A tab bar element. When a new tab is clicked, it can notify the parent
 * element via a callback, or navigate to the link provided in the tab config.
 * Note: this element itself doesn't modify its state. When the selected tab is
 * changed, the parent element must set a new selectedTabId.
 */
@customElement('milo-tab-bar')
export class TabBarElement extends MobxLitElement {
  @observable.ref tabs: TabDef[] = [];
  @observable.ref selectedTabId = '';

  onTabClicked: (clickedTabId: string) => void = () => {};

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    return html`
      ${this.tabs.map(
        (tab) => html`
          <a
            class=${classMap({ tab: true, selected: this.selectedTabId === tab.id })}
            @click=${() => {
              if (this.selectedTabId === tab.id) {
                return;
              }
              this.onTabClicked(this.selectedTabId);
            }}
            href=${tab.href}
            >${tab.label}${tab.slotName ? html` <slot name=${tab.slotName}></slot>` : ''}</a
          >
        `
      )}
    `;
  }

  static styles = css`
    :host {
      display: block;
      height: 25px;
      border-bottom: 1px solid var(--divider-color);
    }

    .tab {
      color: var(--default-text-color);
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
