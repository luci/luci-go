// Copyright 2023 The LUCI Authors.
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
import Hotkeys, { HotkeysEvent, KeyHandler } from 'hotkeys-js';
import { customElement } from 'lit-element';
import { makeObservable, observable } from 'mobx';
import { useEffect } from 'react';
import * as React from 'react';
import { createRoot, Root } from 'react-dom/client';

// Let individual hotkey element set the filters instead.
Hotkeys.filter = () => true;

export interface HotkeyProps {
  readonly hotkey: string;
  readonly handler: KeyHandler;
  readonly filter?: (keyboardEvent: KeyboardEvent, hotkeysEvent: HotkeysEvent) => boolean;
  readonly children: React.ReactNode;
}

/**
 * Register a global keydown event listener.
 * The event listener is automatically unregistered when the component is
 * disconnected.
 */
export function Hotkey({ hotkey, handler, filter, children }: HotkeyProps) {
  const filterFn =
    filter ??
    // By default, prevent hotkeys from reacting to events from input related elements
    // enclosed in shadow DOM.
    ((keyboardEvent: KeyboardEvent, _hotkeysEvent: HotkeysEvent) => {
      const tagName = (keyboardEvent.composedPath()[0] as Partial<HTMLElement>).tagName || '';
      return !['INPUT', 'SELECT', 'TEXTAREA'].includes(tagName);
    });

  const handle: KeyHandler = (keyboardEvent, hotkeysEvent) => {
    if (!filterFn(keyboardEvent, hotkeysEvent)) {
      return;
    }
    handler(keyboardEvent, hotkeysEvent);
  };

  useEffect(() => {
    Hotkeys(hotkey, handle);
    return () => {
      Hotkeys.unbind(hotkey);
    };
  }, [hotkey, handler, filter]);

  return <>{children}</>;
}

@customElement('milo-hotkey')
export class HotkeyElement extends MobxLitElement {
  @observable.ref key!: string;
  handler!: KeyHandler;
  @observable.ref filter?: Extract<HotkeyProps, 'filter'>;

  private readonly parent: HTMLSpanElement;
  private readonly root: Root;

  constructor() {
    super();
    makeObservable(this);
    this.parent = document.createElement('span');
    this.root = createRoot(this.parent);
  }

  protected render() {
    this.root.render(
      <Hotkey hotkey={this.key} handler={this.handler} filter={this.filter}>
        <slot></slot>
      </Hotkey>
    );
    return this.parent;
  }
}
