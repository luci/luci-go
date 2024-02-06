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

import '@material/mwc-icon';
import { MobxLitElement } from '@adobe/lit-mobx';
import copy from 'copy-to-clipboard';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';

/**
 * A simple icon that copies textToCopy to clipboard onclick.
 */
@customElement('milo-copy-to-clipboard')
export class CopyToClipboardElement extends MobxLitElement {
  @observable.ref copied = false;
  // Allow the parent to pass a function so we can avoid pre-generating the text
  // when it's too costly to generate or can change frequently.
  textToCopy: string | (() => string) = '';

  constructor() {
    super();
    makeObservable(this);
    this.addEventListener('click', () => {
      if (this.copied) {
        return;
      }
      copy(
        typeof this.textToCopy === 'function'
          ? this.textToCopy()
          : this.textToCopy,
      );
      this.copied = true;
      setTimeout(() => (this.copied = false), 1000);
    });
  }

  protected render() {
    if (this.copied) {
      return html`
        <slot name="done-icon">
          <mwc-icon>done</mwc-icon>
        </slot>
      `;
    }

    return html`
      <slot name="copy-icon">
        <mwc-icon>content_copy</mwc-icon>
      </slot>
    `;
  }

  static styles = css`
    :host {
      cursor: pointer;
      display: inline-block;
      vertical-align: text-bottom;
      width: 16px;
      height: 16px;
      border-radius: 2px;
      padding: 2px;
      --mdc-icon-size: 16px;
    }
    :host(:hover) {
      background-color: silver;
    }
  `;
}
