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
import { css, customElement, html } from 'lit-element';

import commonStyle from '../styles/common_style.css';

@customElement('milo-not-found-page')
export class NotFoundPageElement extends MobxLitElement {
  protected render() {
    return html` <div id="not-found-message">We couldn't find the page you were looking for.</div> `;
  }

  static styles = [
    commonStyle,
    css`
      #not-found-message {
        margin: 8px 16px;
      }
    `,
  ];
}
