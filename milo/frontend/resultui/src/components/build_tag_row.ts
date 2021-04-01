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

import { css, customElement, LitElement, property } from 'lit-element';
import { html } from 'lit-html';

import { getSafeUrlFromBuildset } from '../libs/build_utils';

/**
 * Renders a build tag row, include linkify support for some build tags.
 */
@customElement('milo-build-tag-row')
export class BuildTagRowElement extends LitElement {
  @property() key!: string;
  @property() value!: string;

  protected render() {
    if (this.key === 'buildset') {
      const url = getSafeUrlFromBuildset(this.value);
      if (url) {
        return html`
          <td>${this.key}</td>
          <td><a href=${url} target="_blank">${this.value}</a></td>
        `;
      }
    }

    return html`
      <td>${this.key}</td>
      <td>${this.value}</td>
    `;
  }

  static styles = css`
    :host {
      display: table-row;
    }

    td:nth-child(2) {
      clear: both;
      overflow-wrap: anywhere;
    }
  `;
}
