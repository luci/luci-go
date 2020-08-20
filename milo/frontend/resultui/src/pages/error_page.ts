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
import { BeforeEnterObserver, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { sanitizeHTML } from '../libs/sanitize_html';


/**
 * Renders sourceUrl and reason from search params (if present).
 */
@customElement('milo-error-page')
export class ErrorPageElement extends MobxLitElement implements BeforeEnterObserver {
  private sourceUrl = '';
  private reason = '';

  onBeforeEnter(location: RouterLocation) {
    const searchParams = new URLSearchParams(location.search);
    this.sourceUrl = searchParams.get('sourceUrl') ?? '';
    this.reason = searchParams.get('reason') ?? 'something went wrong';
  }

  protected renderSourceUrl() {
    // lit-html doesn't fully sanitize href attributes, use sanitizeHTML
    // instead.
    return sanitizeHTML(`<a href="${this.sourceUrl}">${this.sourceUrl}</a>`);
  }

  protected render() {
    return html`
      <div>
        ${this.sourceUrl ?
          html`An error occurred when visiting the following URL:<br>${this.renderSourceUrl()}` :
          'An error occurred:'}
      </div>
      <div id="reason">
        ${this.reason.split('\n').map((line) => html`<p>${line}</p>`)}
      </div>
    `;
  }

  static styles = css`
    :host > div {
      margin: 8px 16px;
    }

    #reason {
      background-color: rgb(245, 245, 245);
      padding: 5px;
    }
  `;
}
