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

import { RpcCode } from '@chopsui/prpc-client';
import stableStringify from 'fast-json-stable-stringify';
import { computed, observable, untracked } from 'mobx';

import { MAY_REQUIRE_SIGNIN } from '../common_tags';
import { createContextLink } from '../libs/context';
import { PrpcClientExt } from '../libs/prpc_client_ext';
import { attachTags } from '../libs/tag';
import { AccessService, BuilderID, BuildersService, BuildsService } from '../services/buildbucket';
import { AuthState, MiloInternal } from '../services/milo_internal';
import { ResultDb } from '../services/resultdb';
import { TestHistoryService } from '../services/test_history_service';

const MAY_REQUIRE_SIGNIN_ERROR_CODE = [RpcCode.NOT_FOUND, RpcCode.PERMISSION_DENIED, RpcCode.UNAUTHENTICATED];

/**
 * Records the app-level state.
 */
export class AppState {
  @observable.ref timestamp = Date.now();

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

  // null means it's not initialized yet.
  // undefined means there's no such service worker.
  @observable.ref redirectSw: ServiceWorkerRegistration | null | undefined = null;

  private cachedBuildId = new Map<string, string>();
  setBuildId(builderId: BuilderID, buildNum: number, buildId: string) {
    this.cachedBuildId = this.cachedBuildId.set(stableStringify([builderId, buildNum]), buildId);
  }
  getBuildId(builderId: BuilderID, buildNum: number) {
    return this.cachedBuildId.get(stableStringify([builderId, buildNum]));
  }

  // Whether the test results tab loading time has been recorded.
  sentTestResultsTabLoadingTimeToGA = false;

  @observable.ref private isDisposed = false;

  @observable.ref authState: AuthState | null = null;

  // null means the userIdentity is uninitialized (i.e. we don't know whether
  // the user is logged in or not).
  @computed get userIdentity() {
    return this.authState?.identity || null;
  }

  @computed({ keepAlive: true })
  get resultDb(): ResultDb | null {
    if (this.isDisposed || this.userIdentity === null) {
      return null;
    }
    return new ResultDb(this.makeClient(CONFIGS.RESULT_DB.HOST));
  }

  @computed({ keepAlive: true })
  get testHistoryService(): TestHistoryService | null {
    if (this.isDisposed || this.resultDb === null) {
      return null;
    }
    return new TestHistoryService(this.resultDb);
  }

  @computed({ keepAlive: true })
  get milo(): MiloInternal | null {
    if (this.isDisposed || this.userIdentity === null) {
      return null;
    }
    return new MiloInternal(this.makeClient(''));
  }

  @computed({ keepAlive: true })
  get buildsService(): BuildsService | null {
    if (this.isDisposed || this.userIdentity === null) {
      return null;
    }
    return new BuildsService(this.makeClient(CONFIGS.BUILDBUCKET.HOST));
  }

  @computed({ keepAlive: true })
  get buildersService(): BuildersService | null {
    if (this.isDisposed || this.userIdentity === null) {
      return null;
    }
    return new BuildersService(this.makeClient(CONFIGS.BUILDBUCKET.HOST));
  }

  @computed({ keepAlive: true })
  get accessService(): AccessService | null {
    if (this.isDisposed || this.userIdentity === null) {
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
    this.milo;
    this.buildsService;
    this.buildersService;
    this.accessService;
  }

  private makeClient(host: string) {
    // Don't track the access token so services won't be refreshed when the
    // access token is updated.
    return new PrpcClientExt(
      { host },
      () => untracked(() => this.authState?.accessToken || ''),
      (e) => {
        if (MAY_REQUIRE_SIGNIN_ERROR_CODE.includes(e.code)) {
          attachTags(e, MAY_REQUIRE_SIGNIN);
        }
        throw e;
      }
    );
  }

  // Refresh all data that depends on the timestamp.
  refresh() {
    this.timestamp = Date.now();
  }
}

export const [provideAppState, consumeAppState] = createContextLink<AppState>();
