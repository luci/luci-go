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

/**
 * A component that supports lazy rendering of a list.
 * It will notifies the child nodes that are entering the view.
 *
 * The child elements should implement OnEnterList and
 * 1. render a placeholder by default.
 * 2. fully render the element when OnEnterList is called.
 *
 * When this.growth is set to > 0, the rendered height will grow by at least
 * this.growth per LazyListElement.MIN_INTERVAL ms.
 *
 * Caveat:
 * The placeholder element should be of equal height of the actual element.
 * 1. If it has a smaller height, some off-view elements might be rendered.
 * 2. If it has a greater height, some in-view elements might not be rendered.
 *
 * If the actual element height can't be determined before rendering, you should
 * set the placeholder height to the lower bound of the possible actual height.
 *
 * List entries should be sorted by offsetTop, which means css styles that can
 * change the position order of the child elements (e.g. "position: fixed;"
 * "transform:") should be used with caution. If an element is positioned before
 * its previous sibling, it may not be rendered until the previous sibling is
 * rendered.
 *
 * Example
 * ```typescript
 * @customElement('custom-entry')
 * class CustomEntryElement extends LitElement implements OnEnterList {
 *   @property() prerender = true
 *
 *   onEnterList() {
 *     this.prerender = false;
 *   }
 *
 *   protected render() {
 *     if (this.prerender) {
 *       // return a placeholder that has roughly the same height as the fully
 *       // rendered component.
 *       return html`<div style="height: 20px;"></div>`
 *     }
 *     return html`
 *       <div style="height: 20px;">
 *         <!-- render the component -->
 *       </div>
 *     `
 *   }
 * }
 *
 * @customElement('custom-list')
 * class CustomListElement extends LitElement {
 *   protected render() {
 *     // only the first ~10 items will be rendered before scrolling.
 *     return html`
 *       <milo-lazy-list style="height: 200px;">
 *         ${new Array(100).fill(0).map(() => html`<custom-entry></custom-entry`)}
 *       </milo-lazy-list>
 *     `
 *   }
 * }
 * ```
 */
@customElement('milo-lazy-list')
export class LazyListElement extends LitElement {
  static readonly MIN_INTERVAL = 100;

  get growth() {
    return this._growth;
  }
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
    return html` <slot id="slot"></slot> `;
  }

  static styles = css`
    :host {
      display: block;
      position: relative;
      overflow-y: auto;
    }
  `;
}
