// Copyright 2024 The LUCI Authors.
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

import { render, screen, fireEvent } from '@testing-library/react';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  emptyPageTokenUpdater,
  getPageToken,
  usePagerContext,
} from './context';
import { ParamsPager } from './params_pager';

function NavigationTestContainer() {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [25, 50, 100, 200],
    defaultPageSize: 50,
    // Keys should be set to default keys when empty string is provided.
    pageSizeKey: '',
    pageTokenKey: '',
  });
  const [searchParams] = useSyncedSearchParams();
  const pageToken = getPageToken(pagerCtx, searchParams);
  const nextPageToken =
    { '': 'page2', page2: 'page3', page3: '' }[pageToken] || '';
  return <ParamsPager pagerCtx={pagerCtx} nextPageToken={nextPageToken} />;
}

function MultiplePagersTestContainer() {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [25, 50, 100, 200],
    defaultPageSize: 50,
  });
  const [searchParams] = useSyncedSearchParams();
  const pageToken = getPageToken(pagerCtx, searchParams);
  const nextPageToken =
    { '': 'page2', page2: 'page3', page3: '' }[pageToken] || '';
  return (
    <>
      <ParamsPager pagerCtx={pagerCtx} nextPageToken={nextPageToken} />
      <ParamsPager pagerCtx={pagerCtx} nextPageToken={nextPageToken} />
    </>
  );
}

function MultipleCustomizeKeysTestContainer() {
  const pagerCtx1 = usePagerContext({
    pageSizeOptions: [25, 50, 100, 200],
    defaultPageSize: 50,
    pageSizeKey: 'q1limit',
    pageTokenKey: 'q1cursor',
  });
  const pagerCtx2 = usePagerContext({
    pageSizeOptions: [25, 50, 100, 200],
    defaultPageSize: 50,
    pageSizeKey: 'q2limit',
    pageTokenKey: 'q2cursor',
  });
  const [searchParams] = useSyncedSearchParams();
  const pageToken1 = getPageToken(pagerCtx1, searchParams);
  const pageToken2 = getPageToken(pagerCtx2, searchParams);
  const nextPageToken1 =
    { '': 'q1page2', q1page2: 'q1page3', q1page3: '' }[pageToken1] || '';
  const nextPageToken2 =
    { '': 'q2page2', q2page2: 'q2page3', q2page3: '' }[pageToken2] || '';
  return (
    <>
      <ParamsPager pagerCtx={pagerCtx1} nextPageToken={nextPageToken1} />
      <ParamsPager pagerCtx={pagerCtx2} nextPageToken={nextPageToken2} />
    </>
  );
}

function EmptyPageTokenTestContainer() {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [25, 50, 100, 200],
    defaultPageSize: 50,
  });
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const pageToken = getPageToken(pagerCtx, searchParams);
  const nextPageToken =
    { '': 'page2', page2: 'page3', page3: '' }[pageToken] || '';

  return (
    <>
      <ParamsPager pagerCtx={pagerCtx} nextPageToken={nextPageToken} />
      <button
        data-testid="empty-button"
        onClick={() => setSearchParams(emptyPageTokenUpdater(pagerCtx))}
      />
    </>
  );
}

describe('ParamsPager', () => {
  it('should navigate between pages properly', async () => {
    render(
      <FakeContextProvider>
        <NavigationTestContainer />
      </FakeContextProvider>,
    );

    const prevPageLink = screen.getByText('Previous Page');
    const nextPageLink = screen.getByText('Next Page');

    expect(prevPageLink).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page2');

    fireEvent.click(nextPageLink);
    expect(prevPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink).toHaveAttribute('href', '/');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page3');

    fireEvent.click(nextPageLink);
    expect(prevPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink).toHaveAttribute('href', '/?cursor=page2');
    expect(nextPageLink).toHaveAttribute('aria-disabled', 'true');

    fireEvent.click(prevPageLink);
    expect(prevPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink).toHaveAttribute('href', '/');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page3');

    fireEvent.click(prevPageLink);
    expect(prevPageLink).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page2');
  });

  it('allows users to go back to the first page when initial page token is provided', async () => {
    render(
      <FakeContextProvider
        routerOptions={{ initialEntries: ['?cursor=page3'] }}
      >
        <NavigationTestContainer />
      </FakeContextProvider>,
    );

    const prevPageLink = screen.getByText('Previous Page');
    const nextPageLink = screen.getByText('Next Page');

    expect(prevPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink).toHaveAttribute('href', '/');
    expect(nextPageLink).toHaveAttribute('aria-disabled', 'true');

    fireEvent.click(prevPageLink);
    expect(prevPageLink).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page2');
  });

  it('should navigate between pages properly when there are multiple pagers', async () => {
    render(
      <FakeContextProvider>
        <MultiplePagersTestContainer />
      </FakeContextProvider>,
    );

    const [prevPageLink1, prevPageLink2] = screen.getAllByText('Previous Page');
    const [nextPageLink1, nextPageLink2] = screen.getAllByText('Next Page');

    expect(prevPageLink1).toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink2).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).toHaveAttribute('href', '/?cursor=page2');
    expect(nextPageLink2).toHaveAttribute('href', '/?cursor=page2');

    fireEvent.click(nextPageLink1);
    expect(prevPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink1).toHaveAttribute('href', '/');
    expect(prevPageLink2).toHaveAttribute('href', '/');
    expect(nextPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).toHaveAttribute('href', '/?cursor=page3');
    expect(nextPageLink2).toHaveAttribute('href', '/?cursor=page3');

    fireEvent.click(nextPageLink2);
    expect(prevPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink1).toHaveAttribute('href', '/?cursor=page2');
    expect(prevPageLink2).toHaveAttribute('href', '/?cursor=page2');
    expect(nextPageLink1).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink2).toHaveAttribute('aria-disabled', 'true');

    fireEvent.click(prevPageLink1);
    expect(prevPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink1).toHaveAttribute('href', '/');
    expect(prevPageLink2).toHaveAttribute('href', '/');
    expect(nextPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).toHaveAttribute('href', '/?cursor=page3');
    expect(nextPageLink2).toHaveAttribute('href', '/?cursor=page3');

    fireEvent.click(prevPageLink2);
    expect(prevPageLink1).toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink2).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).toHaveAttribute('href', '/?cursor=page2');
    expect(nextPageLink2).toHaveAttribute('href', '/?cursor=page2');
  });

  it('emptyPageTokenUpdater should discard prev page tokens', async () => {
    render(
      <FakeContextProvider>
        <EmptyPageTokenTestContainer />
      </FakeContextProvider>,
    );

    const prevPageLink = screen.getByText('Previous Page');
    const nextPageLink = screen.getByText('Next Page');
    const emptyPageTokenButton = screen.getByTestId('empty-button');

    expect(prevPageLink).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page2');

    fireEvent.click(nextPageLink);
    expect(prevPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink).toHaveAttribute('href', '/');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page3');

    fireEvent.click(nextPageLink);
    expect(prevPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink).toHaveAttribute('href', '/?cursor=page2');
    expect(nextPageLink).toHaveAttribute('aria-disabled', 'true');

    fireEvent.click(emptyPageTokenButton);
    expect(prevPageLink).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page2');
  });

  it('can customize pageSizeKey and pageTokenKey', async () => {
    render(
      <FakeContextProvider>
        <MultipleCustomizeKeysTestContainer />
      </FakeContextProvider>,
    );

    const [prevPageLink1, prevPageLink2] = screen.getAllByText('Previous Page');
    const [nextPageLink1, nextPageLink2] = screen.getAllByText('Next Page');

    expect(prevPageLink1).toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink2).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).toHaveAttribute('href', '/?q1cursor=q1page2');
    expect(nextPageLink2).toHaveAttribute('href', '/?q2cursor=q2page2');

    fireEvent.click(nextPageLink1);
    expect(prevPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink1).toHaveAttribute('href', '/');
    expect(nextPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).toHaveAttribute('href', '/?q1cursor=q1page3');
    expect(prevPageLink2).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink2).toHaveAttribute(
      'href',
      '/?q1cursor=q1page2&q2cursor=q2page2',
    );

    fireEvent.click(nextPageLink2);
    expect(prevPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink1).toHaveAttribute('href', '/?q2cursor=q2page2');
    expect(nextPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).toHaveAttribute(
      'href',
      '/?q1cursor=q1page3&q2cursor=q2page2',
    );
    expect(prevPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink2).toHaveAttribute('href', '/?q1cursor=q1page2');
    expect(nextPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink2).toHaveAttribute(
      'href',
      '/?q1cursor=q1page2&q2cursor=q2page3',
    );

    fireEvent.click(prevPageLink1);
    expect(prevPageLink1).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).toHaveAttribute(
      'href',
      '/?q2cursor=q2page2&q1cursor=q1page2',
    );
    expect(prevPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink2).toHaveAttribute('href', '/');
    expect(nextPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink2).toHaveAttribute('href', '/?q2cursor=q2page3');

    fireEvent.click(prevPageLink2);
    expect(prevPageLink1).toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink2).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink2).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink1).toHaveAttribute('href', '/?q1cursor=q1page2');
    expect(nextPageLink2).toHaveAttribute('href', '/?q2cursor=q2page2');
  });
});
