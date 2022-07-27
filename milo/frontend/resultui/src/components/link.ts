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
import { customElement } from 'lit-element';
import { html } from 'lit-html';
import { makeObservable, observable } from 'mobx';

import { Link } from '../models/link';
import commonStyle from '../styles/common_style.css';

/**
 * Renders a Link object.
 */
@customElement('milo-link')
export class LinkElement extends MobxLitElement {
  @observable.ref link!: Link;
  @observable.ref target?: string;

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    return html`
      <a href=${this.link.url} aria-label=${this.link.ariaLabel} target=${this.target || ''}>${this.link.label}</a>
    `;
  }

  static styles = [commonStyle];
}
