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
import { customElement, html } from 'lit-element';
import { observable } from 'mobx';

/**
 * A simple element that re-renders the component automatically when observable
 * properties accessed during renderFn are updated.
 *
 * This can be used to build simple components that react to update events so
 * the parent element doesn't need to subscribe to the same update events. As a
 * result, the parent element (which can be more expensive to render) doesn't
 * need to be re-rendered as frequently.
 */
@customElement('milo-autorun')
export class FrequentUpdateElement extends MobxLitElement {
  @observable.ref renderFn = () => html``;

  protected render() {
    return this.renderFn();
  }
}
