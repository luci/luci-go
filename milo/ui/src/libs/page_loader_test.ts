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

import { expect } from '@jest/globals';

import { PageLoader } from './page_loader';

const pages: { [pageToken: string]: [items: number[], nextPageToken?: string] } = {
  ['page1']: [[1, 2, 3, 4], 'page2'],
  ['page2']: [[5, 6, 7, 8], 'page3'],
  ['page3']: [[9, 10]],
};

describe('PageLoader', () => {
  it('e2e', async () => {
    const pageLoader = new PageLoader(async (pageToken) => pages[pageToken || 'page1']);

    expect(pageLoader.loadedFirstPage).toBeFalsy();
    expect(pageLoader.loadedAll).toBeFalsy();
    expect(pageLoader.items).toEqual([]);

    const page1 = await pageLoader.loadNextPage();
    expect(page1).toEqual(pages['page1'][0]);
    expect(pageLoader.loadedFirstPage).toBeTruthy();
    expect(pageLoader.loadedAll).toBeFalsy();
    expect(pageLoader.items).toEqual([1, 2, 3, 4]);

    const page2 = await pageLoader.loadNextPage();
    expect(page2).toEqual(pages['page2'][0]);
    expect(pageLoader.loadedFirstPage).toBeTruthy();
    expect(pageLoader.loadedAll).toBeFalsy();
    expect(pageLoader.items).toEqual([1, 2, 3, 4, 5, 6, 7, 8]);

    const page3 = await pageLoader.loadNextPage();
    expect(page3).toEqual(pages['page3'][0]);
    expect(pageLoader.loadedFirstPage).toBeTruthy();
    expect(pageLoader.loadedAll).toBeTruthy();
    expect(pageLoader.items).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);

    // Page 4 doesn't exist.
    const page4 = await pageLoader.loadNextPage();
    expect(page4).toBeNull();
    expect(pageLoader.loadedFirstPage).toBeTruthy();
    expect(pageLoader.loadedAll).toBeTruthy();
    expect(pageLoader.items).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
  });

  describe('loadFirstPage works', () => {
    it('when called first', async () => {
      const pageLoader = new PageLoader(async (pageToken) => pages[pageToken || 'page1']);

      const page1 = await pageLoader.loadFirstPage();
      expect(page1).toEqual(pages['page1'][0]);

      // Calling loadNextPage after calling loadFirstPage should load the
      // subsequent page.
      const page2 = await pageLoader.loadNextPage();
      expect(page2).toEqual(pages['page2'][0]);

      // After loading other pages, loadFirstPage still returns the first page.
      const firstPageAgain = await pageLoader.loadFirstPage();
      expect(firstPageAgain).toEqual(pages['page1'][0]);
    });

    it('when called after loading other pages', async () => {
      const pageLoader = new PageLoader(async (pageToken) => pages[pageToken || 'page1']);

      const page1 = await pageLoader.loadNextPage();
      expect(page1).toEqual(pages['page1'][0]);

      const firstPageAgain = await pageLoader.loadFirstPage();
      expect(firstPageAgain).toEqual(pages['page1'][0]);

      // Calling loadFirstPage when the first page is already loaded should not
      // load the next page. Therefore, a second loadNextPage call should load
      // the second page.
      const page2 = await pageLoader.loadNextPage();
      expect(page2).toEqual(pages['page2'][0]);
    });
  });

  describe('isLoading is populated correctly', () => {
    it('when loading pages sequentially', async () => {
      const pageLoader = new PageLoader(async (pageToken) => pages[pageToken || 'page1']);

      const page1Promise = pageLoader.loadNextPage();
      expect(pageLoader.isLoading).toBeTruthy();
      await page1Promise;
      expect(pageLoader.isLoading).toBeFalsy();

      const page2Promise = pageLoader.loadNextPage();
      expect(pageLoader.isLoading).toBeTruthy();
      await page2Promise;
      expect(pageLoader.isLoading).toBeFalsy();

      const page3Promise = pageLoader.loadNextPage();
      expect(pageLoader.isLoading).toBeTruthy();
      await page3Promise;
      expect(pageLoader.isLoading).toBeFalsy();

      const page4Promise = pageLoader.loadNextPage();
      // Page 4 doesn't exist.
      expect(pageLoader.isLoading).toBeFalsy();
      await page4Promise;
      expect(pageLoader.isLoading).toBeFalsy();
    });

    it('when loading pages in parallel', async () => {
      const pageLoader = new PageLoader(async (pageToken) => pages[pageToken || 'page1']);

      const page1Promise = pageLoader.loadNextPage();
      const page2Promise = pageLoader.loadNextPage();
      const page3Promise = pageLoader.loadNextPage();
      const page4Promise = pageLoader.loadNextPage();
      expect(pageLoader.isLoading).toBeTruthy();

      await page1Promise;
      expect(pageLoader.isLoading).toBeTruthy();

      expect(pageLoader.isLoading).toBeTruthy();
      await page2Promise;

      await page3Promise;
      expect(pageLoader.isLoading).toBeFalsy();

      // Page 4 doesn't exist.
      await page4Promise;
      expect(pageLoader.isLoading).toBeFalsy();
    });
  });
});
