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
 * Renders page header, including a sign-in widget.
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
      <img id="chromium-icon" src="/static/assets/chromium-icon.png"/>
      <span id="headline">LUCI Test Results</span>
      <chops-signin id="signin" client-id=${this.clientId}></chops-signin>
    `;
  }

  static styles = css`
    :host {
        padding-left: 5px;
    }
    #chromium-icon {
        display: inline-block;
        width: 50px;
        vertical-align: middle;
    }
    #headline {
        font-size: 34px;
        letter-spacing: 0.25px;
        vertical-align: middle;
    }
    #signin {
        float: right;
        padding: 5px;
    }
  `;
}
