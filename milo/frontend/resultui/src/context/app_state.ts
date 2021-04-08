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

import { computed, observable } from 'mobx';

import { consumeContext, provideContext } from '../libs/context';
import { PrpcClientExt } from '../libs/prpc_client_ext';
import { AccessService, BuildersService, BuildsService } from '../services/buildbucket';
import { MiloInternal } from '../services/milo_internal';
import { ResultDb, UISpecificService } from '../services/resultdb';

/**
 * Records the app-level state.
 */
export class AppState {
  // Don't make this an observable so services won't be refreshed after updating
  // the access token.
  accessToken = '';
  // null means the userId is uninitialized (i.e. we don't know whether
  // the user is logged in or not).
  // '' means the user is not logged in.
  @observable.ref userId: string | null = null;

  // The timestamp when the user selected the current tab.
  tabSelectionTime = TIME_ORIGIN;
  @observable.ref _selectedTabId: string | null = null;
  get selectedTabId() {
    return this._selectedTabId || '';
  }
  set selectedTabId(newTabId: string) {
    // Don't update the tab selection time when selected tab is being
    // initialized. The page load time should be used in that case.
    if (this._selectedTabId !== null) {
      this.tabSelectionTime = Date.now();
    }
    this._selectedTabId = newTabId;
  }

  @observable.ref selectedBlamelistPinIndex = 0;

  // Use number instead of boolean because previousPage.disconnectedCallback
  // might be called after currentPage.disconnectedCallback.
  // Number allows us to reset the setting even when the execution is out of
  // order.
  /**
   * When hasSettingsDialog > 0, the current page has a settings dialog.
   *
   * If the page has a setting dialog, it should increment the number on connect
   * and decrement it on disconnet.
   */
  @observable.ref hasSettingsDialog = 0;
  @observable.ref showSettingsDialog = false;

  @observable.ref gAuth: gapi.auth2.GoogleAuth | null = null;

  // null means it's not initialized yet.
  // undefined means there's no such service worker.
  @observable.ref redirectSw: ServiceWorkerRegistration | null | undefined = null;

  @observable.ref private isDisposed = false;

  @computed({ keepAlive: true })
  get resultDb(): ResultDb | null {
    if (this.isDisposed || this.userId === null) {
      return null;
    }
    return new ResultDb(this.makeClient(CONFIGS.RESULT_DB.HOST));
  }

  @computed({ keepAlive: true })
  get uiSpecificService(): UISpecificService | null {
    if (this.isDisposed || this.userId === null) {
      return null;
    }
    return new UISpecificService(this.makeClient(CONFIGS.RESULT_DB.HOST));
  }

  @computed({ keepAlive: true })
  get milo(): MiloInternal | null {
    if (this.isDisposed || this.userId === null) {
      return null;
    }
    return new MiloInternal(this.makeClient(''));
  }

  @computed({ keepAlive: true })
  get buildsService(): BuildsService | null {
    if (this.isDisposed || this.userId === null) {
      return null;
    }
    return new BuildsService(this.makeClient(CONFIGS.BUILDBUCKET.HOST));
  }

  @computed({ keepAlive: true })
  get buildersService(): BuildersService | null {
    if (this.isDisposed || this.userId === null) {
      return null;
    }
    return new BuildersService(this.makeClient(CONFIGS.BUILDBUCKET.HOST));
  }

  @computed({ keepAlive: true })
  get accessService(): AccessService | null {
    if (this.isDisposed || this.userId === null) {
      return null;
    }
    return new AccessService(this.makeClient(CONFIGS.BUILDBUCKET.HOST));
  }

  /**
   * Perform cleanup.
   * Must be called before the object is GCed.
   */
  dispose() {
    this.isDisposed = true;

    // Evaluates @computed({keepAlive: true}) properties after this.isDisposed
    // is set to true so they no longer subscribes to any external observable.
    this.resultDb;
    this.uiSpecificService;
    this.milo;
    this.buildsService;
    this.buildersService;
    this.accessService;
  }

  private makeClient(host: string) {
    return new PrpcClientExt({ host }, () => this.accessToken);
  }
}

export const consumeAppState = consumeContext<'appState', AppState>('appState');
export const provideAppState = provideContext<'appState', AppState>('appState');
