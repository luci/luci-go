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
import { html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';

export interface ConnectionEventDetail<T> {
  data: T;
  addDisconnectedCB: (cb: (data: T) => void) => void;
}

export type ConnectionEvent<T> = CustomEvent<ConnectionEventDetail<T>>;

/**
 * Emits the a ConnectionEvent with the specified event type and data when
 * connected to DOM.
 * Disconnect event listener can be added via
 * event.detail.addDisconnectedEventCB
 */
@customElement('milo-connection-observer')
export class ConnectionObserverElement<T> extends MobxLitElement {
  static get properties() {
    return {
      eventType: {
        attribute: 'event-type',
        type: String,
      },
      data: {
        type: Object,
      },
    };
  }

  set eventType(newVal: string) {
    this._eventType = newVal;
  }

  set data(newVal: T) {
    this._data = newVal;
  }

  @observable.ref _eventType = 'connected';
  @observable.ref _data!: T;

  disconnectedListeners: Array<(data: T) => void> = [];

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();
    this.dispatchEvent(
      new CustomEvent<ConnectionEventDetail<T>>(this._eventType, {
        bubbles: true,
        composed: true,
        detail: {
          data: this._data,
          addDisconnectedCB: (cb: (data: T) => void) => {
            this.disconnectedListeners.push(cb);
          },
        },
      })
    );
  }

  disconnectedCallback() {
    for (const cb of this.disconnectedListeners) {
      cb(this._data);
    }
    super.disconnectedCallback();
  }

  protected render() {
    return html`<slot></slot>`;
  }
}
