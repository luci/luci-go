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

import BottleNeck from 'bottleneck';
import { css, customElement, LitElement } from 'lit-element';
import { html } from 'lit-html';

export interface OnEnterList extends HTMLElement {
  onEnterList?(): void;
}

@customElement('milo-lazy-list')
export class LazyListElement extends LitElement {
  static readonly MIN_INTERVAL = 100;

  get growth() { return this._growth; }
  set growth(newVal: number) {
    this._growth = newVal;
    if (this.limiter.empty()) {
      this.renderTo(this.renderedHeight + this.growth);
    }
  }
  private _growth = 0;

  private elements: OnEnterList[] = [];
  private pivot = 0;
  private slotEle!: HTMLSlotElement;

  private limiter = new BottleNeck({
    maxConcurrent: 1,
    minTime: LazyListElement.MIN_INTERVAL,
    highWater: 2,
    strategy: BottleNeck.strategy.OVERFLOW,
    rejectOnDrop: false,
  });

  private renderedHeight = 0;

  /**
   * Renders the list to the specified height.
   * Then starts the rendering loop.
   */
  private renderTo(height: number) {
    // If the target height was reached, or all items have been rendered,
    // stop the rendering loop.
    if (height <= this.renderedHeight || this.pivot >= this.elements.length) {
      return;
    }
    this.renderedHeight = height;

    this.limiter.schedule(async () => {
      for (; this.pivot < this.elements.length; this.pivot++) {
        const ele = this.elements[this.pivot];
        if (ele.offsetTop > this.renderedHeight) {
          break;
        }
        ele.onEnterList?.();
      }
      this.renderTo(this.renderedHeight + this.growth);
    });
  }

  protected firstUpdated() {
    this.slotEle = this.shadowRoot!.getElementById('slot') as HTMLSlotElement;
    this.init();
    this.slotEle.addEventListener('slotchange', () => this.init());
  }

  /**
   * Resets the rendering state then starts the rendering loop again.
   */
  private async init() {
    this.elements = this.slotEle.assignedElements() as OnEnterList[];
    this.pivot = 0;
    this.renderedHeight = 0;
    this.onscroll();
  }

  onscroll = () => this.renderTo(this.clientHeight + this.scrollTop);

  protected render() {
    return html`
      <slot id="slot"></slot>
    `;
  }

  static styles = css`
    :host {
      display: block;
      position: relative;
      overflow-y: auto;
    }
  `;
}
