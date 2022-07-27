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

import Hotkeys, { HotkeysEvent, KeyHandler } from 'hotkeys-js';
import { customElement, html } from 'lit-element';
import { makeObservable, observable, reaction } from 'mobx';

import { MiloBaseElement } from './milo_base';

// Let individual hotkey element set the filters instead.
Hotkeys.filter = () => true;

/**
 * Register a global keydown event listener.
 * The event listener is automatically unregistered when the component is
 * disconnected.
 */
@customElement('milo-hotkey')
export class HotkeyElement extends MiloBaseElement {
  @observable.ref key!: string;
  handler!: KeyHandler;

  // By default, prevent hotkeys from reacting to events from input related elements
  // enclosed in shadow DOM.
  filter = (keyboardEvent: KeyboardEvent, _hotkeysEvent: HotkeysEvent) => {
    const tagName = (keyboardEvent.composedPath()[0] as Partial<HTMLElement>).tagName || '';
    return !['INPUT', 'SELECT', 'TEXTAREA'].includes(tagName);
  };

  // Use _ prefix to prevent typo when assigning property in lit-html template.
  private readonly _handle = (keyboardEvent: KeyboardEvent, hotkeysEvent: HotkeysEvent) => {
    if (!this.filter(keyboardEvent, hotkeysEvent)) {
      return;
    }
    this.handler(keyboardEvent, hotkeysEvent);
  };

  private oldKey?: string;

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => [this.key],
        ([key]) => {
          if (!this.isConnected) {
            return false;
          }

          if (this.oldKey) {
            Hotkeys.unbind(this.oldKey, this._handle);
            this.oldKey = '';
          }
          if (key) {
            this.oldKey = key;
            Hotkeys(key, this._handle);
          }
          return true;
        },
        { fireImmediately: true }
      )
    );
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    if (this.oldKey) {
      Hotkeys.unbind(this.oldKey, this._handle);
      this.oldKey = '';
    }
  }

  protected render() {
    return html`<slot></slot>`;
  }
}
