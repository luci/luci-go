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
import { classMap } from 'lit/directives/class-map.js';
import { styleMap } from 'lit/directives/style-map.js';
import { makeObservable, observable } from 'mobx';

export interface Component {
  color: string;
  weight: number;
}

@customElement('milo-status-bar')
export class StatusBarElement extends MobxLitElement {
  @observable.ref
  components: Component[] = [];

  @observable.ref
  loading = false;

  constructor() {
    super();
    makeObservable(this);
  }

  private renderComponent(component: Component) {
    return html`
      <span
        class=${classMap({ loading: this.loading })}
        style=${styleMap({
          'flex-grow': component.weight.toString(),
          'background-color': component.color,
        })}
      >
      </span>
    `;
  }

  protected render() {
    return html`
      <div id="container">
        ${this.components.map((c) => this.renderComponent(c))}
      </div>
    `;
  }

  static styles = css`
    #container {
      display: flex;
      height: 5px;
    }

    .loading {
      background-image: linear-gradient(
        45deg,
        rgba(255, 255, 255, 0.15) 25%,
        transparent 25%,
        transparent 50%,
        rgba(255, 255, 255, 0.15) 50%,
        rgba(255, 255, 255, 0.15) 75%,
        transparent 75%,
        transparent
      );
      background-size: 1rem 1rem;
      animation: progress-bar-stripes 1s linear infinite;
    }

    @keyframes progress-bar-stripes {
      0% {
        background-position: 1rem 0;
      }
      100% {
        background-position: 0 0;
      }
    }
  `;
}
