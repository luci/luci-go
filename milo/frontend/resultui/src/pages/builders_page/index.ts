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

import { MobxLitElement } from '@adobe/lit-mobx';
import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { observable } from 'mobx';

import '../../components/status_bar';
import { AppState, consumeAppState } from '../../context/app_state';
import { getURLPathForProject } from '../../libs/build_utils';
import { consumer } from '../../libs/context';
import { NOT_FOUND_URL } from '../../routes';
import commonStyle from '../../styles/common_style.css';

@customElement('milo-builders-page')
@consumer
export class BuildersPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref @consumeAppState() appState!: AppState;

  private project!: string;
  private group!: string;

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

  protected render() {
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
      <milo-status-bar .components=${[{ color: 'var(--active-color)', weight: 1 }]}></milo-status-bar>
    `;
  }

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
    `,
  ];
}
