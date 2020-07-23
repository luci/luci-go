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
import '@material/mwc-icon';
import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { computed, observable } from 'mobx';

import '../../components/status_bar';
import '../../components/tab_bar';
import { TabDef } from '../../components/tab_bar';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { NOT_FOUND_URL, router } from '../../routes';

/**
 * Main build page.
 * Reads project, bucket, builder and build from URL params.
 * If any of the parameters are not provided, redirects to '/not-found'.
 * If build is not a number, shows an error.
 */
export class BuildPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;

  private project = '';
  private bucket = '';
  private builder = '';
  private buildNumOrId = '';

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const project = location.params['project'];
    const bucket = location.params['bucket'];
    const builder = location.params['builder'];
    const buildNumOrId = location.params['build_num_or_id'];
    if ([project, bucket, builder, buildNumOrId].some((param) => typeof param !== 'string')) {
      return cmd.redirect(NOT_FOUND_URL);
    }

    this.project = project as string;
    this.bucket = bucket as string;
    this.builder = builder as string;
    this.buildNumOrId = buildNumOrId as string;
    return;
  }

  @computed get tabDefs(): TabDef[] {
    return [
      {
        id: 'overview',
        label: 'Overview',
        href: router.urlForName(
          'build',
          {
            'project': this.project,
            'bucket': this.bucket,
            'builder': this.builder,
            'build_num_or_id': this.buildNumOrId,
          },
        ),
      },
    ];
  }

  protected render() {
    return html`
      <div id="build-summary">
        <div id="build-id">
          <span id="build-id-label">Build</span>
          <span>${this.builder} / ${this.buildNumOrId}</span>
        </div>
      </div>
      <tr-status-bar
        .components=${[{color: '#007bff', weight: 1}]}
      ></tr-status-bar>
      <tr-tab-bar
        .tabs=${this.tabDefs}
        .selectedTabId=${'overview'}
      ></tr-tab-bar>
      <slot></slot>
    `;
  }

  static styles = css`
    :host {
      height: calc(100vh - var(--header-height));
      display: grid;
      grid-template-rows: repeat(3, auto) 1fr;
    }

    #build-summary {
      background-color: rgb(248, 249, 250);
      padding: 6px 16px;
      font-family: "Google Sans", "Helvetica Neue", sans-serif;
      font-size: 14px;
      display: flex;
    }

    #build-id {
      flex: 0 auto;
    }

    #build-id-label {
      color: rgb(95, 99, 104);
    }

    #build-state {
      margin-left: auto;
      flex: 0 auto;
    }
  `;
}

customElement('tr-build-page')(
  consumeAppState(
    BuildPageElement,
  ),
);
