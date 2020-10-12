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

import Hotkeys, { KeyHandler } from 'hotkeys-js';
import { customElement, html, LitElement, property, PropertyValues } from 'lit-element';


/**
 * Register a global keydown event listener.
 * The event listener is automatically unregistered when the component is
 * disconnected.
 */
@customElement('milo-hotkey')
export class HotkeyElement extends LitElement {
  @property() key!: string;
  handler!: KeyHandler;

  private handle: KeyHandler = (...params) => this.handler(...params);

  shouldUpdate(changedProperties: PropertyValues) {
    if (!this.isConnected) {
      return false;
    }

    const oldKey = changedProperties.get('key') as string | undefined;
    if (oldKey) {
      Hotkeys.unbind(oldKey, this.handle);
      Hotkeys(this.key, this.handle);
    }
    return true;
  }

  connectedCallback() {
    super.connectedCallback();
    Hotkeys(this.key, this.handle);
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    Hotkeys.unbind(this.key, this.handle);
  }

  protected render() {
    return html`<slot></slot>`;
  }
}
