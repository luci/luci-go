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

import '@chopsui/chops-signin';

import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement, html } from 'lit-element';
import { observable } from 'mobx';

/**
 * Renders page header, including a sign-in widget, at the top of the child
 * nodes.
 */
@customElement('tr-page-header')
export class PageHeaderElement extends MobxLitElement {
  // TODO(weiweilin): load the clientId from somewhere instead of hard-coding
  // it.
  @observable
      .ref clientId =
      '897369734084-d3t2c39aht2aqeop0f42pp48ejpr54up.apps.googleusercontent.com';

  protected render() {
    return html`
      <div id="container">
        <div id="title-container">
          <img id="chromium-icon" src="https://storage.googleapis.com/chrome-infra/lucy-small.png"/>
          <span id="headline">LUCI Test Results (BETA)</span>
        </div>
        <chops-signin id="signin" client-id=${this.clientId}></chops-signin>
      </div>
      <slot></slot>
    `;
  }

  static styles = css`
    :host {
      --header-height: 52px;
    }

    #container {
      box-sizing: border-box;
      height: var(--header-height);
      padding: 10px 0;
      display: flex;
    }
    #title-container {
      display: flex;
      flex: 1 1 100%;
      align-items: center;
      margin-left: 14px;
    }
    #chromium-icon {
      display: inline-block;
      width: 32px;
      height: 32px;
      margin-right: 8px;
    }
    #headline {
      color: rgb(95, 99, 104);
      font-family: "Google Sans", "Helvetica Neue", sans-serif;
      font-size: 18px;
      font-weight: 300;
      letter-spacing: 0.25px;
    }
    #signin {
        margin-right: 14px;
    }
  `;
}
