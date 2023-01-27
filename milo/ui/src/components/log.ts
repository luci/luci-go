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
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { styleMap } from 'lit/directives/style-map.js';
import { makeObservable, observable } from 'mobx';

import { getLogdogRawUrl } from '../libs/build_utils';
import { Log } from '../services/buildbucket';

/**
 * Renders a Log object.
 */
@customElement('milo-log')
export class LogElement extends MobxLitElement {
  @observable.ref log!: Log;

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    return html`
      <a href=${this.log.viewUrl} target="_blank">${this.log.name}</a>
      <span style=${styleMap({ display: ['stdout', 'stderr'].indexOf(this.log.name) !== -1 ? '' : 'none' })}>
        [<a href=${getLogdogRawUrl(this.log.url)} target="_blank">raw</a>]
      </span>
    `;
  }

  static styles = css`
    a {
      color: var(--default-text-color);
    }
  `;
}
