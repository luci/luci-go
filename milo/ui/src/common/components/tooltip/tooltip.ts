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

import { MobxLitElement } from '@adobe/lit-mobx';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { action, makeObservable, observable } from 'mobx';

export interface ShowTooltipEventDetail {
  tooltip: HTMLElement;
  // The location around which the tooltip should be displayed.
  targetRect: DOMRectReadOnly;
  // The gap between the tooltip and the targetRect.
  gapSize: number;
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
export class TooltipElement extends MobxLitElement {
  @observable.ref private tooltip?: HTMLElement;
  @observable.ref private targetRect?: DOMRectReadOnly;
  @observable.ref private gapSize?: number;

  private hideTooltipTimeout = 0;

  private onShowTooltip = action((event: Event) => {
    window.clearTimeout(this.hideTooltipTimeout);

    const e = event as CustomEvent<ShowTooltipEventDetail>;
    this.tooltip = e.detail.tooltip;
    this.targetRect = e.detail.targetRect;
    this.gapSize = e.detail.gapSize;

    this.style.display = 'block';

    // Hide the element until we decided where to render it.
    this.style.visibility = 'hidden';

    // Reset the position so it's easier to calculate the new position.
    this.style.left = '0';
    this.style.top = '0';
  });

  private onHideTooltip = (event: Event) => {
    window.clearTimeout(this.hideTooltipTimeout);

    const e = event as CustomEvent<HideTooltipEventDetail>;
    this.hideTooltipTimeout = window.setTimeout(
      this.hideTooltip,
      e.detail.delay || 0,
    );
  };

  private hideTooltip = action(() => {
    this.style.display = 'none';
    this.tooltip = undefined;
    this.targetRect = undefined;
    this.gapSize = undefined;
  });

  constructor() {
    super();
    makeObservable(this);
    this.addEventListener('mouseover', () =>
      window.clearTimeout(this.hideTooltipTimeout),
    );
    this.addEventListener('mouseout', this.hideTooltip);
  }

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
    if (!this.tooltip || !this.targetRect || this.gapSize === undefined) {
      return;
    }

    const selfRect = this.getBoundingClientRect();

    const offsets = [
      // Bottom (left-aligned).
      [
        this.targetRect.left + window.scrollX,
        this.targetRect.bottom + this.gapSize + window.scrollY,
      ],
      // Bottom (right-aligned).
      [
        this.targetRect.right - selfRect.width + window.scrollX,
        this.targetRect.bottom + this.gapSize + window.scrollY,
      ],
      // Top (left-aligned).
      [
        this.targetRect.left + window.scrollX,
        this.targetRect.top - selfRect.height - this.gapSize + window.scrollY,
      ],
      // Top (right-aligned).
      [
        this.targetRect.right - selfRect.width + window.scrollX,
        this.targetRect.top - selfRect.height - this.gapSize + window.scrollY,
      ],
      // Right (top-aligned).
      [
        this.targetRect.right + this.gapSize + window.scrollX,
        this.targetRect.top + window.scrollY,
      ],
      // Right (bottom-aligned).
      [
        this.targetRect.right + this.gapSize + window.scrollX,
        this.targetRect.bottom - selfRect.height + window.scrollY,
      ],
      // Left (top-aligned).
      [
        this.targetRect.left - selfRect.width - this.gapSize + window.scrollX,
        this.targetRect.top + window.scrollY,
      ],
      // Left (bottom-aligned).
      [
        this.targetRect.left - selfRect.width - this.gapSize + window.scrollX,
        this.targetRect.bottom - selfRect.height + window.scrollY,
      ],
      // Bottom-right.
      [
        this.targetRect.right + this.gapSize + window.scrollX,
        this.targetRect.bottom + this.gapSize + window.scrollY,
      ],
      // Bottom-left.
      [
        this.targetRect.left - selfRect.width - this.gapSize + window.scrollX,
        this.targetRect.bottom + this.gapSize + window.scrollY,
      ],
      // Top-right.
      [
        this.targetRect.right + this.gapSize + window.scrollX,
        this.targetRect.top - selfRect.height - this.gapSize + window.scrollY,
      ],
      // Top-left.
      [
        this.targetRect.left - selfRect.width - this.gapSize + window.scrollX,
        this.targetRect.top - selfRect.height - this.gapSize + window.scrollY,
      ],
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
      box-shadow:
        rgb(0 0 0 / 20%) 0px 5px 5px -3px,
        rgb(0 0 0 / 14%) 0px 8px 10px 1px,
        rgb(0 0 0 / 12%) 0px 3px 14px 2px;
      z-index: 999;
    }
  `;
}

declare module 'react' {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-tooltip': Record<string, never>;
    }
  }
}
