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

import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { repeat } from 'lit/directives/repeat.js';
import { styleMap } from 'lit/directives/style-map.js';
import { computed, makeObservable, observable, reaction } from 'mobx';
import { useEffect } from 'react';
import { useParams } from 'react-router-dom';

import '../../components/status_bar';
import '../../components/dot_spinner';
import './row';
import { MiloBaseElement } from '../../components/milo_base';
import { consumer, provider } from '../../libs/context';
import { reportError, reportErrorAsync } from '../../libs/error_handler';
import {
  IntersectionNotifier,
  provideNotifier,
} from '../../libs/observer_element';
import { getProjectURLPath } from '../../libs/url_utils';
import { BuilderID } from '../../services/buildbucket';
import {
  ListBuildersRequest,
  ListBuildersResponse,
} from '../../services/milo_internal';
import { consumeStore, StoreInstance } from '../../store';
import { commonStyles } from '../../styles/stylesheets';

@customElement('milo-builders-page')
@provider
@consumer
export class BuildersPageElement extends MiloBaseElement {
  static get properties() {
    return {
      project: {
        type: String,
      },
      group: {
        type: String,
      },
    };
  }

  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @provideNotifier()
  notifier = new IntersectionNotifier({ rootMargin: '1000px' });

  @observable.ref _project!: string;
  @computed get project() {
    return this._project;
  }
  set project(newVal: string) {
    this._project = newVal;
  }

  @observable.ref _group!: string;
  @computed get group() {
    return this._group;
  }
  set group(newVal: string) {
    this._group = newVal;
  }

  @observable.ref private numOfBuilds = 25;
  @observable.ref private builders: readonly BuilderID[] = [];
  @observable.ref private isLoading = false;

  @computed
  private get listBuildersResIter(): AsyncIterableIterator<ListBuildersResponse> {
    if (!this.store.services.milo) {
      return (async function* () {
        yield Promise.race([]);
      })();
    }

    let req: ListBuildersRequest = {
      project: this.project,
      group: this.group,
    };
    const milo = this.store.services.milo;

    async function* streamListBuildersRes() {
      let res: ListBuildersResponse;
      do {
        res = await milo.listBuilders(req);
        req = { ...req, pageToken: res.nextPageToken };
        yield res;
      } while (res.nextPageToken);
    }

    return streamListBuildersRes();
  }

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback(): void {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.listBuildersResIter,
        () => {
          this.builders = [];
          this.loadAllPages();
        },
        { fireImmediately: true }
      )
    );
  }

  private loadAllPages = reportErrorAsync(this, async () => {
    this.isLoading = true;
    for await (const buildersRes of this.listBuildersResIter) {
      this.builders = this.builders.concat(
        buildersRes.builders?.map((v) => v.id) || []
      );
    }
    this.isLoading = false;
  });

  protected render = reportError(this, () => {
    return html`
      <div id="builders-group-id">
        <a href=${getProjectURLPath(this.project)}>${this.project}</a>
        ${
          this.group
            ? html`
                <span>&nbsp;/&nbsp;</span>
                <span>group</span>
                <span>&nbsp;/&nbsp;</span>
                <span>${this.group}</span>
              `
            : ''
        }
        <span>&nbsp;/&nbsp;</span><span>builders</span>
      </div>
      <milo-status-bar
        .components=${[{ color: 'var(--active-color)', weight: 1 }]}
        .isLoading=${this.isLoading}
      ></milo-status-bar>
      <div id="main">
        <table>
          <tbody>
            ${repeat(
              this.builders,
              (b) => b.project + '/' + b.bucket + '/' + b.builder,
              (b) =>
                html`<milo-builders-page-row
                  .builder=${b}
                  .numOfBuilds=${this.numOfBuilds}
                ></milo-builders-page-row>`
            )}
          </tbody>
        </table>

        <div id="loading-row">
          <span>Showing ${this.builders.length} builders.</span>
          <span style=${styleMap({ display: this.isLoading ? '' : 'none' })}>
            Loading <milo-dot-spinner></milo-dot-spinner>
          </span>
          </span>
          <br />
          <span>
            Number of builds per builder (10-100):
            <input
              id="num-of-builds"
              type="number"
              min="10"
              max="100"
              value=${this.numOfBuilds}
              @change=${(e: InputEvent) => {
                this.numOfBuilds = Number((e.target as HTMLInputElement).value);
              }}
            />
          </span>
        </div>
      </div>
    `;
  });

  static styles = [
    commonStyles,
    css`
      #builders-group-id {
        background-color: var(--block-background-color);
        padding: 6px 16px;
        font-family: 'Google Sans', 'Helvetica Neue', sans-serif;
        font-size: 14px;
        display: flex;
      }

      #main {
        margin-top: 5px;
        margin-left: 10px;
      }

      milo-builders-page-row:nth-child(odd) {
        background-color: var(--block-background-color);
      }

      table {
        width: 100%;
      }

      #loading-row {
        margin-top: 5px;
      }
      #load {
        color: var(--active-text-color);
      }
      #load-more {
        color: var(--active-text-color);
        cursor: pointer;
      }
    `,
  ];
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-builders-page': {
        project: string;
        group: string;
      };
    }
  }
}

export function BuildersPage() {
  const { project, group } = useParams();

  if (!project) {
    throw new Error('invariant violated: project should be set');
  }

  useEffect(() => {
    document.title = (group || project) + ' | Builders';
  }, [project, group]);

  return (
    <milo-builders-page
      project={project}
      group={group || ''}
    ></milo-builders-page>
  );
}
