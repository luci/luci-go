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
import { computed, makeObservable, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import '../../components/dot_spinner';
import { MiloBaseElement } from '../../components/milo_base';
import { getURLPathForBuild, getURLPathForBuilder } from '../../libs/build_utils';
import { BUILD_STATUS_CLASS_MAP } from '../../libs/constants';
import { reportRenderError } from '../../libs/error_handler';
import { lazyRendering } from '../../libs/observer_element';
import { unwrapObservable } from '../../libs/unwrap_observable';
import { Build, BuilderID } from '../../services/buildbucket';
import { BuilderStats } from '../../services/milo_internal';
import { consumeStore, StoreInstance } from '../../store';
import commonStyle from '../../styles/common_style.css';

@customElement('milo-builders-page-row')
@lazyRendering
export class BuildersPageRowElement extends MiloBaseElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref builder!: BuilderID;
  @observable.ref numOfBuilds = 25;

  @computed private get builderLink() {
    return getURLPathForBuilder(this.builder);
  }

  @computed private get recentBuilds$(): IPromiseBasedObservable<readonly Build[]> {
    if (!this.store?.services.milo) {
      return fromPromise(Promise.race([]));
    }

    return fromPromise(
      this.store.services.milo
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

  @computed private get builderStats$(): IPromiseBasedObservable<BuilderStats> {
    if (!this.store?.services.milo) {
      return fromPromise(Promise.race([]));
    }

    return fromPromise(
      this.store.services.milo.queryBuilderStats({
        builder: this.builder,
      })
    );
  }

  @computed private get builderStats() {
    return unwrapObservable<BuilderStats | null>(this.builderStats$, null);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  renderPlaceHolder() {
    return html`
      <td class="shrink-to-fit">
        <a href=${this.builderLink}>${this.builder.project}/${this.builder.bucket}/${this.builder.builder}</a>
      </td>
      <td class="shrink-to-fit"></td>
      <td></td>
    `;
  }

  // Do not use the `protected render = reportRenderError(this, () => {...}`
  // shorthand because this method will be overridden by the `@lazyRendering`.
  protected render() {
    return reportRenderError(this, () => {
      return html`
        <td class="shrink-to-fit">
          <a href=${this.builderLink}>${this.builder.project}/${this.builder.bucket}/${this.builder.builder}</a>
        </td>
        <td class="shrink-to-fit">${this.renderBuilderStats()}</td>
        <td>${this.renderRecentBuilds()}</td>
      `;
    })();
  }

  private renderBuilderStats() {
    if (!this.builderStats) {
      return html`<milo-dot-spinner></milo-dot-spinner>`;
    }
    return html`
      <a href=${this.builderLink} class="stats-badge pending-cell">
        ${this.builderStats.pendingBuildsCount || 0} pending
      </a>
      <a href=${this.builderLink} class="stats-badge running-cell">
        ${this.builderStats.runningBuildsCount || 0} running
      </a>
    `;
  }

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

      .shrink-to-fit {
        width: 1px;
        white-space: nowrap;
      }

      .stats-badge {
        font-size: 10px;
        display: inline-block;
        width: 55px;
        height: 20px;
        line-height: 20px;
        text-align: center;
        border: 1px solid black;
        border-radius: 3px;
        text-decoration: none;
        color: var(--default-text-color);
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
    `,
  ];
}
