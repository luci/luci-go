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

/**
 * Renders the child components in an overlay. Fire a 'dismiss' event when users
 * clicks on the background area.
 */
@customElement('milo-overlay')
export class OverlayElement extends LitElement {
  @property({ type: Boolean }) show = false;

  protected render() {
    if (!this.show) {
      return html``;
    }

    return html`
      <div id="overlay" @click=${() => this.dispatchEvent(new Event('dismiss'))}>
        <slot @click=${(e: Event) => e.stopImmediatePropagation()}></slot>
      </div>
    `;
  }

  static styles = css`
    #overlay {
      width: 100vw;
      height: 100vh;
      position: absolute;
      top: 0px;
      background: rgb(0 0 0 / 50%);
      z-index: 2;
    }
  `;
}
