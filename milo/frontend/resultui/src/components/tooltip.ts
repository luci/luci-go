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

const GAP_SIZE = 5;

export interface ShowTooltipEventDetail {
  tooltip: HTMLElement;
  // The location around which the tooltip should be displayed.
  targetRect: DOMRectReadOnly;
}

export type ShowTooltipEvent = CustomEvent<ShowTooltipEventDetail>;

export interface HideTooltipEventDetail {
  // Hide the tooltip after `delay` ms. Default value is 0.
  // When the tooltip is lingering, you can hover over it to stop it from
  // disappearing.
  delay?: number;
}

export type HideTooltipEvent = CustomEvent<HideTooltipEventDetail>;

/**
 * A global listener for displaying instant tooltip. It should be added to
 * somewhere close to the root of the DOM tree.
 *
 * After mounting this to DOM, you can
 * 1. show a tooltip via 'show-tooltip' event.
 * 2. hide the tooltip via 'hide-tooltip' event.
 */
// Comparing to a sub-component, a global tooltip implementation
// 1. makes it easier to ensure there's at most one active tooltip.
// 2. is not constrained by ancestors' overflow setting.
@customElement('milo-tooltip')
export class TooltipElement extends LitElement {
  @property() private tooltip?: HTMLElement;
  @property() private targetRect?: DOMRectReadOnly;

  private hideTooltipTimeout = 0;

  private onShowTooltip = (event: Event) => {
    window.clearTimeout(this.hideTooltipTimeout);

    const e = event as CustomEvent<ShowTooltipEventDetail>;
    this.tooltip = e.detail.tooltip;
    this.targetRect = e.detail.targetRect;

    this.style.display = 'block';

    // Hide the element until we decided where to render it.
    this.style.visibility = 'hidden';

    // Reset the position so it's easier to calculate the new position.
    this.style.left = '0';
    this.style.top = '0';
  };

  private onHideTooltip = (event: Event) => {
    window.clearTimeout(this.hideTooltipTimeout);

    const e = event as CustomEvent<HideTooltipEventDetail>;
    this.hideTooltipTimeout = window.setTimeout(this.hideTooltip, e.detail.delay || 0);
  };

  private hideTooltip = () => {
    this.style.display = 'none';
    this.tooltip = undefined;
    this.targetRect = undefined;
  };

  onmouseover = () => window.clearTimeout(this.hideTooltipTimeout);
  onmouseout = this.hideTooltip;

  connectedCallback() {
    super.connectedCallback();
    window.addEventListener('show-tooltip', this.onShowTooltip);
    window.addEventListener('hide-tooltip', this.onHideTooltip);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('show-tooltip', this.onShowTooltip);
    window.removeEventListener('hide-tooltip', this.onHideTooltip);
  }

  protected render() {
    return html`${this.tooltip}`;
  }

  protected updated() {
    if (!this.tooltip || !this.targetRect) {
      return;
    }

    const selfRect = this.getBoundingClientRect();

    const offsets = [
      // Bottom.
      [this.targetRect.left, this.targetRect.bottom + GAP_SIZE],
      // Top.
      [this.targetRect.left, this.targetRect.top - selfRect.height - GAP_SIZE],
      // Right.
      [this.targetRect.right + GAP_SIZE, this.targetRect.top],
      // Left.
      [this.targetRect.left - selfRect.width - GAP_SIZE, this.targetRect.top],
    ];

    // Show the tooltip at the bottom by default.
    let selectedOffset = offsets[0];

    // Find a place that can render the tooltip without overflowing the browser
    // window.
    for (const [dx, dy] of offsets) {
      if (dx + selfRect.left < 0) {
        continue;
      }
      if (dx + selfRect.right > window.innerWidth) {
        continue;
      }
      if (dy + selfRect.top < 0) {
        continue;
      }
      if (dy + selfRect.bottom > window.innerHeight) {
        continue;
      }
      selectedOffset = [dx, dy];
      break;
    }

    this.style.left = selectedOffset[0] + 'px';
    this.style.top = selectedOffset[1] + 'px';
    this.style.visibility = 'visible';
  }

  static styles = css`
    :host {
      display: none;
      position: absolute;
      background: white;
      border-radius: 4px;
      padding: 5px;
      box-shadow: rgb(0 0 0 / 20%) 0px 5px 5px -3px, rgb(0 0 0 / 14%) 0px 8px 10px 1px,
        rgb(0 0 0 / 12%) 0px 3px 14px 2px;
      z-index: 2;
    }
  `;
}
