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
import { autorun, computed, observable } from 'mobx';

import '../../components/status_bar';
import '../../components/tab_bar';
import { TabDef } from '../../components/tab_bar';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { consumeInvocationState, InvocationState } from '../../context/invocation_state/invocation_state';
import { NOT_FOUND_URL, router } from '../../routes';
import { BuilderID } from '../../services/buildbucket';

/**
 * Main build page.
 * Reads project, bucket, builder and build from URL params.
 * If any of the parameters are not provided, redirects to '/not-found'.
 * If build is not a number, shows an error.
 */
export class BuildPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;
  @observable.ref invocationState!: InvocationState;

  private builder!: BuilderID;
  private build = 0;

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const project = location.params['project'];
    const bucket = location.params['bucket'];
    const builder = location.params['builder'];
    const buildStr = location.params['build'];
    if ([project, bucket, builder, buildStr].some((param) => typeof param !== 'string')) {
      return cmd.redirect(NOT_FOUND_URL);
    }

    this.builder = {
      project: project as string,
      bucket: bucket as string,
      builder: builder as string,
    };
    this.build = Number(buildStr);
    return;
  }

  private disposers: Array<() => void> = [];

  connectedCallback() {
    super.connectedCallback();
    if (isNaN((this.build))) {
      this.dispatchEvent(new ErrorEvent('error', {
        message: 'build ID is not a number',
        composed: true,
        bubbles: true,
      }));
    }
    this.buildState.builder = this.builder;
    this.buildState.buildNumber = this.build;

    this.disposers.push(autorun(
      () => this.invocationState.invocationId = this.buildState.buildPageData
        ?.infra.resultdb.invocation.slice('invocations/'.length) || '',
    ));
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
  }

  @computed get tabDefs(): TabDef[] {
    return [
      {
        id: 'test-results',
        label: 'Test Results',
        href: router.urlForName(
          'build-test-results',
          {
            'project': this.builder.project,
            'bucket': this.builder.bucket,
            'builder': this.builder.builder,
            'build': this.build.toString(),
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
          <span>${this.builder.builder} / ${this.build}</span>
        </div>
      </div>
      <tr-status-bar
        .components=${[{color: '#007bff', weight: 1}]}
      ></tr-status-bar>
      <tr-tab-bar
        .tabs=${this.tabDefs}
        .selectedTabId=${'test-results'}
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
  consumeInvocationState(
    consumeBuildState(
      consumeAppState(
        BuildPageElement,
      ),
    ),
  ),
);
