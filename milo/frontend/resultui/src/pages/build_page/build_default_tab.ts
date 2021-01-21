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

import '@material/mwc-button';
import { Router } from '@vaadin/router';
import { customElement, html, LitElement } from 'lit-element';

import '../../components/commit_entry';
import '../../components/dot_spinner';
import '../../components/hotkey';
import { consumeConfigsStore, UserConfigsStore } from '../../context/app_state/user_configs';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { router } from '../../routes';

export class BuildDefaultTabElement extends LitElement {
  configsStore!: UserConfigsStore;
  buildState!: BuildState;

  connectedCallback() {
    super.connectedCallback();
    const newUrl = router.urlForName(this.configsStore.userConfigs.defaultBuildPageTabName, {
      ...this.buildState.builder!,
      build_num_or_id: this.buildState.buildNumOrId!,
    });

    // Prevent the router from pushing the history state.
    window.history.replaceState({path: newUrl}, '', newUrl);
    Router.go(newUrl);
  }

  protected render() {
    return html``;
  }
}

customElement('milo-build-default-tab')(
  consumeConfigsStore(
    consumeBuildState(
      BuildDefaultTabElement,
    ),
  ),
);
