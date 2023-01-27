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
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';

import { getSafeUrlFromTagValue } from '../libs/build_utils';
import commonStyle from '../styles/common_style.css';

/**
 * Renders a build tag row, include linkify support for some build tags.
 */
@customElement('milo-build-tag-row')
export class BuildTagRowElement extends MobxLitElement {
  @observable.ref key!: string;
  @observable.ref value!: string;

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    const url = getSafeUrlFromTagValue(this.value);
    if (url) {
      return html`
        <td>${this.key}:</td>
        <td><a href=${url} target="_blank">${this.value}</a></td>
      `;
    }

    return html`
      <td>${this.key}:</td>
      <td>${this.value}</td>
    `;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: table-row;
      }

      td:nth-child(2) {
        clear: both;
        overflow-wrap: anywhere;
      }
    `,
  ];
}
