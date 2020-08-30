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

import { AppState, consumeAppState } from '../../context/app_state/app_state';

export class StepsTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'steps';
  }

  protected render() {
    return html`
      <div>This is the steps & logs tab<div>
    `;
  }
}

customElement('milo-steps-tab')(
  consumeAppState(StepsTabElement),
);
