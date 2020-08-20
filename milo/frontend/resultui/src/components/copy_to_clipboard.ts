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
import copy from 'copy-to-clipboard';
import { css, customElement, html } from 'lit-element';
import { observable } from 'mobx';


/**
 * A simple icon that copies textToCopy to clipboard onclick.
 * Size can be configured via --size, defaults to 16px;
 */
@customElement('milo-copy-to-clipboard')
export class CopyToClipboard extends MobxLitElement {
  @observable.ref copied = false;
  textToCopy = '';

  onclick = () => {
    copy(this.textToCopy);
    this.copied = true;
  }

  onmouseleave = () => {
    this.copied = false;
  }

  protected render() {
    return html`
      <svg
        class="inline-icon"
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 24 24"
      >
        <path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/>
      </svg>
      <span ?hidden=${!this.copied}>copied</span>
    `;
  }

  static styles = css`
    :host {
      cursor: pointer;
    }
    svg {
      width: var(--size, 16px);
      height: var(--size, 16px);
    }
    svg:hover {
      filter: drop-shadow(3px 3px 4px gray);
    }
    span {
      background-color: black;
      color: white;
      padding: 3px;
    }
  `;
}
