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

import { ParamsPager } from './params_pager';
import { getPageToken } from './params_pager_utils';

const ParamsPagerTestContainer = () => {
  const [searchParams, _] = useSyncedSearchParams();
  const pageToken = getPageToken(searchParams);
  const nextPageToken =
    { '': 'page2', page2: 'page3', page3: '' }[pageToken] || '';
  return <ParamsPager nextPageToken={nextPageToken} />;
};
describe('ParamsPager', () => {
  it('should navigate between pages properly', async () => {
    render(
      <FakeContextProvider>
        <ParamsPagerTestContainer />
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
});
