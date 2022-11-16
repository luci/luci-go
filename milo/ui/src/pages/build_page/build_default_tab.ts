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
import '../../components/hotkey';
import { consumer } from '../../libs/context';
import { router } from '../../routes';
import { consumeStore, StoreInstance } from '../../store';

@customElement('milo-build-default-tab')
@consumer
export class BuildDefaultTabElement extends LitElement {
  @consumeStore() store!: StoreInstance;

  connectedCallback() {
    super.connectedCallback();
    const newUrl = router.urlForName(this.store.userConfig.build.defaultTabName, {
      ...this.store.buildPage.builderIdParam!,
      build_num_or_id: this.store.buildPage.buildNumOrIdParam!,
    });

    // Prevent the router from pushing the history state.
    window.history.replaceState(null, '', newUrl);
    // Trigger the new route after the component is connected, otherwise
    // parent components will be mounted again.
    window.setTimeout(() => Router.go(newUrl));
  }

  protected render() {
    return html``;
  }
}
