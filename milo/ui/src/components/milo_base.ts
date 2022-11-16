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

/**
 * A base element that provides addDisposer method.
 */
// We cannot use HTML native event handlers to implement this behavior,
// because dispatching events in disconnectedCallback in no-op.
export abstract class MiloBaseElement extends MobxLitElement {
  private disposers: Array<() => void> = [];

  /**
   * Add a disposer that will be run when the component is disconnected.
   * Newer disposers will be run first.
   * Disposers will be cleared after being run.
   * This function should not be called after calling this.disconnectedCallback.
   */
  protected addDisposer(disposer: () => void) {
    this.disposers.push(disposer);
  }

  disconnectedCallback() {
    // Disposers should be called in reverse order because disposers can have
    // dependencies on disposers that were added earlier.
    this.disposers.reverse().forEach((disposer) => disposer());
    this.disposers = [];
    super.disconnectedCallback();
  }
}
