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

import { customElement, LitElement, property } from 'lit-element';
import { html } from 'lit-html';

import { styleMap } from 'lit-html/directives/style-map';
import { getLogdogRawUrl } from '../libs/build_utils';
import { Log } from '../services/buildbucket';

/**
 * Renders a Log object.
 */
@customElement('milo-log')
export class LogElement extends LitElement {
  @property() log!: Log;

  protected render() {
    return html`
      <a href=${this.log.viewUrl} target="_blank">${this.log.name}</a>
      [<a
        style=${styleMap({'display': ['stdout', 'stderr'].indexOf(this.log.name) !== -1 ? '' : 'none'})}
        href=${getLogdogRawUrl(this.log.url)}
        target="_blank"
      >raw</a>]
    `;
  }
}
