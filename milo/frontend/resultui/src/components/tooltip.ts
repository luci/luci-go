// Copyright 2021 The LUCI Authors.
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

import { css, customElement, html, LitElement, property } from 'lit-element';

export interface ShowTooltipEventDetail {
  tooltip: HTMLElement;
  position: {
    x: number;
    y: number;
  };
}

export type ShowTooltipEvent = CustomEvent<ShowTooltipEventDetail>;

/**
 * Shows a tooltip after 'show-tooltip' event is dispatched.
 * Hide the tooltip after 'hide-tooltip' even is dispatched.
 */
@customElement('milo-tooltip')
export class TooltipElement extends LitElement {
  @property() private tooltip?: HTMLElement;

  private showTooltip = (event: Event) => {
    const e = event as CustomEvent<ShowTooltipEventDetail>;
    this.tooltip = e.detail.tooltip;
    this.style.display = 'block';
    this.style.left = e.detail.position.x + 'px';
    this.style.top = e.detail.position.y + 'px';
  }

  private hideTooltip = () => {
    this.style.display = 'none';
    this.tooltip = undefined;
  }

  connectedCallback() {
    super.connectedCallback();
    window.addEventListener('show-tooltip', this.showTooltip);
    window.addEventListener('hide-tooltip', this.hideTooltip);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('show-tooltip', this.showTooltip);
    window.removeEventListener('hide-tooltip', this.hideTooltip);
  }

  protected render() {
    return html`${this.tooltip}`;
  }

  static styles = css`
    :host {
      display: none;
      position: absolute;
      background: white;
      border-radius: 4px;
      padding: 5px;
      box-shadow:
        rgb(0 0 0 / 20%) 0px 5px 5px -3px,
        rgb(0 0 0 / 14%) 0px 8px 10px 1px,
        rgb(0 0 0 / 12%) 0px 3px 14px 2px;
      z-index: 2;
    }
  `;
}
