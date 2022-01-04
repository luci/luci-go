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

import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable, reaction } from 'mobx';

import '../../components/status_bar';
import '../../components/dot_spinner';
import './row';
import { MiloBaseElement } from '../../components/milo_base';
import { AppState, consumeAppState } from '../../context/app_state';
import { getURLPathForProject } from '../../libs/build_utils';
import { consumer } from '../../libs/context';
import { reportError, reportErrorAsync } from '../../libs/error_handler';
import { NOT_FOUND_URL } from '../../routes';
import { BuilderID } from '../../services/buildbucket';
import { ListBuildersRequest, ListBuildersResponse } from '../../services/milo_internal';
import commonStyle from '../../styles/common_style.css';

@customElement('milo-builders-page')
@consumer
export class BuildersPageElement extends MiloBaseElement implements BeforeEnterObserver {
  @observable.ref @consumeAppState() appState!: AppState;

  private project!: string;
  private group!: string;

  @observable.ref private builders: readonly BuilderID[] = [];
  @observable.ref private isLoading = false;
  @observable.ref private endOfPage = true;

  @computed private get listBuildersResIter(): AsyncIterableIterator<ListBuildersResponse> {
    if (!this.appState.milo) {
      return (async function* () {
        yield Promise.race([]);
      })();
    }

    let req: ListBuildersRequest = {
      project: this.project,
      group: this.group,
    };
    const milo = this.appState.milo;

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

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const project = location.params['project'];
    const group = location.params['group'] || '';

    if ([project, group].some((param) => typeof param !== 'string')) {
      return cmd.redirect(NOT_FOUND_URL);
    }

    this.project = project as string;
    this.group = group as string;

    document.title = (this.group || this.project) + ' | Builders';

    return;
  }

  connectedCallback(): void {
    super.connectedCallback();

    reaction(
      () => this.listBuildersResIter,
      () => {
        this.builders = [];
        this.loadNextPage();
      },
      { fireImmediately: true }
    );
  }

  private loadNextPage = reportErrorAsync(this, async () => {
    this.isLoading = true;
    this.endOfPage = false;
    const next = await this.listBuildersResIter.next();
    if (next.done) {
      this.endOfPage = true;
    } else {
      this.builders = this.builders.concat(next.value.builders?.map((v) => v.id) || []);
      this.endOfPage = !next.value.nextPageToken;
    }
    this.isLoading = false;
  });

  protected render = reportError(this, () => {
    return html`
      <div id="builders-group-id">
        <a href=${getURLPathForProject(this.project)}>${this.project}</a>
        ${this.group
          ? html`
              <span>&nbsp;/&nbsp;</span>
              <span>group</span>
              <span>&nbsp;/&nbsp;</span>
              <span>${this.group}</span>
            `
          : ''}
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
              (b) => html`<milo-builders-page-row .builder=${b}></milo-builders-page-row>`
            )}
          </tbody>
        </table>

        <div id="loading-row">
          <span>Showing ${this.builders.length} builders.</span>
          <span id="load" style=${styleMap({ display: this.endOfPage ? 'none' : '' })}>
            <span
              id="load-more"
              style=${styleMap({ display: this.isLoading ? 'none' : '' })}
              @click=${this.loadNextPage}
            >
              Load More
            </span>
            <span style=${styleMap({ display: this.isLoading ? '' : 'none' })}>
              Loading <milo-dot-spinner></milo-dot-spinner>
            </span>
          </span>
        </div>
      </div>
    `;
  });

  static styles = [
    commonStyle,
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
