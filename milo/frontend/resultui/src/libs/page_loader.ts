// Copyright 2022 The LUCI Authors.
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

import { action, makeObservable, observable } from 'mobx';

export class PageLoader<T> {
  @observable.shallow
  private readonly _items: T[] = [];
  get items(): readonly T[] {
    return this._items;
  }

  @observable.ref
  private loadingReqCount = 0;
  get isLoading(): boolean {
    return !this._loadedAll && this.loadingReqCount !== 0;
  }

  @observable.ref
  private _loadedAll = false;
  get loadedAll(): boolean {
    return this._loadedAll;
  }

  @observable.ref
  private _loadedFirstPage = false;
  get loadedFirstPage(): boolean {
    return this._loadedFirstPage;
  }

  private pageToken?: string;
  private loadPromise = Promise.resolve<readonly T[] | null>(null);
  private firstLoadPromise?: Promise<readonly T[]>;

  constructor(
    private readonly getNextPage: (pageToken?: string) => Promise<[items: readonly T[], nextPageToken?: string]>
  ) {
    makeObservable(this);
  }

  loadFirstPage() {
    return this.firstLoadPromise || this.loadNextPage();
  }

  loadNextPage() {
    if (this.loadedAll) {
      return Promise.resolve(null);
    }

    action(() => this.loadingReqCount++)();
    this.loadPromise = this.loadPromise
      .then(() => this.loadNextPageInternal())
      .finally(
        action(() => {
          this.loadingReqCount--;
        })
      );
    if (!this.firstLoadPromise) {
      // Cast `readonly T[] | null` to `readonly T[]` because the first page
      // can't be null.
      this.firstLoadPromise = this.loadPromise as Promise<readonly T[]>;
    }

    return this.loadPromise;
  }

  private async loadNextPageInternal() {
    if (this.loadedAll) {
      return null;
    }

    const [items, nextPageToken] = await this.getNextPage(this.pageToken);
    action(() => {
      this._items.push(...items);
      this.pageToken = nextPageToken;
      this._loadedFirstPage = true;
      if (!nextPageToken) {
        this._loadedAll = true;
      }
    })();
    return items;
  }
}
