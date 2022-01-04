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
import { computed, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import '../../components/dot_spinner';
import { MiloBaseElement } from '../../components/milo_base';
import { AppState, consumeAppState } from '../../context/app_state';
import { getURLPathForBuild, getURLPathForBuilder } from '../../libs/build_utils';
import { BUILD_STATUS_CLASS_MAP } from '../../libs/constants';
import { consumer } from '../../libs/context';
import { reportRenderError } from '../../libs/error_handler';
import { unwrapObservable } from '../../libs/unwrap_observable';
import { Build, BuilderID } from '../../services/buildbucket';
import commonStyle from '../../styles/common_style.css';

@customElement('milo-builders-page-row')
@consumer
export class BuildersPageRowElement extends MiloBaseElement {
  @observable.ref @consumeAppState() appState!: AppState;

  @observable.ref builder!: BuilderID;
  @observable.ref numOfBuilds = 25;

  @computed private get recentBuilds$(): IPromiseBasedObservable<readonly Build[]> {
    if (!this.appState?.milo) {
      return fromPromise(Promise.race([]));
    }

    return fromPromise(
      this.appState.milo
        .queryRecentBuilds({
          builder: this.builder,
          pageSize: this.numOfBuilds,
        })
        .then((res) => res.builds || [])
    );
  }

  @computed private get recentBuilds() {
    return unwrapObservable<readonly Build[] | null>(this.recentBuilds$, null);
  }

  protected render = reportRenderError(this, () => {
    return html`
      <td class="builder-id">
        <a href=${getURLPathForBuilder(this.builder)}>
          ${this.builder.project}/${this.builder.bucket}/${this.builder.builder}
        </a>
      </td>
      <td>${this.renderRecentBuilds()}</td>
    `;
  });

  private renderRecentBuilds() {
    if (!this.recentBuilds) {
      return html`<milo-dot-spinner></milo-dot-spinner>`;
    }
    return html`
      <div id="builds">
        ${Array(this.numOfBuilds)
          .fill(0)
          .map((_, i) => {
            const build = this.recentBuilds?.[i];
            if (!build) {
              return html`<a class="cell"></a>`;
            }
            return html`
              <a
                class="cell build ${BUILD_STATUS_CLASS_MAP[build.status]}-cell"
                href=${getURLPathForBuild(build)}
                target="_blank"
              ></a>
            `;
          })}
      </div>
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

      .builder-id {
        width: 1px;
        white-space: nowrap;
      }

      #builds {
        display: flex;
      }

      .cell {
        display: inline-block;
        visibility: hidden;
        flex-grow: 1;
        height: 20px;
        line-height: 20px;
        margin: 0;
        text-align: center;
        border: 1px solid black;
        border-radius: 3px;
      }
      .cell.build {
        visibility: visible;
        text-decoration: none;
        color: var(--default-text-color);
      }

      .cell:not(:first-child) {
        border-left: 0px;
      }
      .cell:first-child:after {
        content: 'latest';
      }

      /*
       * The new build page background color doesn't have enough contrast
       * when used as cell background color.
       * Use the original milo color for now.
       */
      .pending-cell {
        background-color: #ccc;
      }
      .running-cell {
        background-color: #fd3;
      }
      .success-cell {
        background-color: #8d4;
      }
      .failure-cell {
        background-color: #e88;
      }
      .infra-failure-cell {
        background-color: #c6c;
      }
      .canceled-cell {
        background-color: #8ef;
      }

      .cell.infra-failure-cell {
        color: white;
      }

      milo-dot-spinner {
        color: var(--active-text-color);
      }
    `,
  ];
}
