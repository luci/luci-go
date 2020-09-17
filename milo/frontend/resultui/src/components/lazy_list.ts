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

import { css, customElement, LitElement } from 'lit-element';
import { html } from 'lit-html';
import { waitUntilNextFrame } from '../libs/utils';

export interface OnEnterList extends HTMLElement {
  onEnterList?(): void;
}

@customElement('milo-lazy-list')
export class LazyListElement extends LitElement {
  private elements: OnEnterList[] = [];
  private pivot = 0;
  private slotEle!: HTMLSlotElement;

  protected render() {
    console.log(1);
    return html`
      <slot id="slot"></slot>
    `;
  }

  protected firstUpdated() {
    this.slotEle = this.shadowRoot!.getElementById('slot') as HTMLSlotElement;
    this.init();
    this.slotEle.addEventListener('slotchange', () => this.init());
  }

  private async init() {
    console.log('init');
    this.elements = this.slotEle.assignedElements() as OnEnterList[];
    // this.pivot = 0;
    await waitUntilNextFrame();
    this.notifyEnterList();
  }

  private notifyEnterList() {
    const viewLimit = this.clientHeight + this.scrollTop + 500;
    if (!this.isConnected) {
      return;
    }
    for (; this.pivot < this.elements.length; this.pivot++) {
      const ele = this.elements[this.pivot];
      if (ele.offsetTop > viewLimit) {
        break;
      }
      console.log('notify');
      ele.onEnterList?.();
    }
  }

  private ticking = false;
  onscroll = () => {
    if (this.ticking) {
      return;
    }
    this.ticking = true;
    setTimeout(() => this.ticking = false, 100);
    this.notifyEnterList();
  }

  static styles = css`
    :host {
      display: block;
      position: relative;
    }
  `;
}
