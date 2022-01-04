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

import { css, customElement, html } from 'lit-element';

import { MiloBaseElement } from '../../components/milo_base';
import { getURLPathForBuilder } from '../../libs/build_utils';
import { BuilderID } from '../../services/buildbucket';
import commonStyle from '../../styles/common_style.css';

@customElement('milo-builders-page-row')
export class BuildersPageRowElement extends MiloBaseElement {
  builder!: BuilderID;

  protected render() {
    return html`
      <td>
        <a href=${getURLPathForBuilder(this.builder)}>
          ${this.builder.project}/${this.builder.bucket}/${this.builder.builder}
        </a>
      </td>
    `;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: table-row;
        width: 100%;
        height: 40px;
        vertical-align: middle;
      }

      td {
        padding: 5px;
      }
    `,
  ];
}
