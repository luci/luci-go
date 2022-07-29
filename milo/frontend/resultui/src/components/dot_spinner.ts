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

import { css, customElement, html, LitElement } from 'lit-element';

/**
 * A simple 3-dots loading indicator.
 */
@customElement('milo-dot-spinner')
export class DotSpinnerElement extends LitElement {
  protected render() {
    /* eslint-disable-next-line */
    return html`<div></div><div></div><div></div>`;
  }

  static styles = css`
    :host {
      display: inline-block;
      text-align: center;
      color: var(--active-text-color);
    }

    div {
      width: 0.75em;
      height: 0.75em;
      border-radius: 100%;
      background-color: currentColor;
      display: inline-block;
      animation: bounce 1.4s infinite ease-in-out both;
    }

    div:nth-child(1) {
      animation-delay: -0.32s;
    }

    div:nth-child(2) {
      animation-delay: -0.16s;
    }

    @keyframes bounce {
      0%,
      80%,
      100% {
        transform: scale(0);
      }
      40% {
        transform: scale(1);
      }
    }
  `;
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-dot-spinner': {};
    }
  }
}
