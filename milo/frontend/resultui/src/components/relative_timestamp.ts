// Copyright 2022 The LUCI Authors.
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

import { customElement, html } from 'lit-element';
import { DateTime, Duration } from 'luxon';
import { autorun, computed, makeObservable, observable } from 'mobx';

import { displayDuration } from '../libs/time_utils';
import { MiloBaseElement } from './milo_base';

/**
 * An element that shows the specified duration relative to now.
 */
@customElement('milo-relative-timestamp')
export class RelativeTimestampElement extends MiloBaseElement {
  @observable.ref timestamp!: DateTime;

  // Interval between updates. In ms.
  @observable.ref interval = 1000;
  @observable.ref formatFn = displayDuration;

  private intervalHandle = 0;

  @observable.ref private duration!: Duration;

  private updateDuration = () => {
    this.duration = this.timestamp.diffNow();
  };

  @computed get relativeTimestamp() {
    if (this.duration.toMillis() > 0) {
      return 'in ' + this.formatFn(this.duration);
    } else {
      return this.formatFn(this.duration.negate()) + ' ago';
    }
  }

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();

    this.updateDuration();

    this.addDisposer(
      autorun(() => {
        clearInterval(this.intervalHandle);
        this.intervalHandle = window.setInterval(this.updateDuration, this.interval);
      })
    );

    this.addDisposer(() => clearInterval(this.intervalHandle));
  }

  protected render() {
    return html`<span>${this.relativeTimestamp}</span>`;
  }
}
