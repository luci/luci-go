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

import { html, LitElement } from 'lit';
import { customElement } from 'lit/decorators.js';

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
export class ConnectionObserverElement<T> extends LitElement {
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

  private _eventType = 'connected';
  get eventType() {
    return this._eventType;
  }
  set eventType(newVal: string) {
    this._eventType = newVal;
  }

  private _data!: T;
  get data() {
    return this._data;
  }
  set data(newVal: T) {
    this._data = newVal;
  }

  disconnectedListeners: Array<(data: T) => void> = [];

  connectedCallback() {
    super.connectedCallback();
    this.dispatchEvent(
      new CustomEvent<ConnectionEventDetail<T>>(this.eventType, {
        bubbles: true,
        composed: true,
        detail: {
          data: this.data,
          addDisconnectedCB: (cb: (data: T) => void) => {
            this.disconnectedListeners.push(cb);
          },
        },
      })
    );
  }

  disconnectedCallback() {
    for (const cb of this.disconnectedListeners) {
      cb(this.data);
    }
    super.disconnectedCallback();
  }

  protected render() {
    return html`<slot></slot>`;
  }
}
