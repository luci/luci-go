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

import { fireEvent, render, screen } from '@testing-library/react';
import { Link } from 'react-router-dom';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ContentGroup } from '../content_group';
import { TrackSearchParamKeys } from '../track_search_param_keys';

import { TrackLeafRoutePageView } from './track_leaf_route_page_view';

describe('TrackLeafRoutePageView', () => {
  let gtagSpy: jest.SpiedFunction<typeof gtag>;

  beforeEach(() => {
    jest.useFakeTimers();
    gtagSpy = jest.spyOn(self, 'gtag').mockImplementation(() => {});
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it('combine content groups correctly', async () => {
    render(
      <FakeContextProvider>
        <ContentGroup group="grandparent">
          <ContentGroup group="parent">
            <TrackLeafRoutePageView contentGroup="self">
              <div>content</div>
            </TrackLeafRoutePageView>
          </ContentGroup>
        </ContentGroup>
      </FakeContextProvider>,
    );

    expect(gtagSpy).toHaveBeenCalledTimes(1);
    expect(gtagSpy).toHaveBeenLastCalledWith(
      'event',
      'page_view',
      expect.objectContaining({ content_group: 'grandparent | parent | self' }),
    );
  });

  it('works without ancestor content groups', async () => {
    render(
      <FakeContextProvider>
        <TrackLeafRoutePageView contentGroup="self">
          <div>content</div>
        </TrackLeafRoutePageView>
      </FakeContextProvider>,
    );

    expect(gtagSpy).toHaveBeenCalledTimes(1);
    expect(gtagSpy).toHaveBeenLastCalledWith(
      'event',
      'page_view',
      expect.objectContaining({ content_group: 'self' }),
    );
  });

  it('only track specified search param keys', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: [
            '/?key1=val1&key2=val2&untracked_key=val3&key2=val4',
          ],
        }}
      >
        <TrackLeafRoutePageView
          contentGroup="self"
          searchParamKeys={['key1', 'key2']}
        >
          <div>content</div>
        </TrackLeafRoutePageView>
      </FakeContextProvider>,
    );

    expect(gtagSpy).toHaveBeenCalledTimes(1);
    expect(gtagSpy).toHaveBeenLastCalledWith(
      'event',
      'page_view',
      expect.objectContaining({
        page_location: 'http://localhost/?key1=val1&key2=val2&key2=val4',
      }),
    );
  });

  it('combine tracked search param keys correctly', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: [
            '/?key1=val1&key2=val2&untracked_key=val3&key2=val4',
          ],
        }}
      >
        <TrackSearchParamKeys keys={['key1']}>
          <TrackLeafRoutePageView
            contentGroup="self"
            searchParamKeys={['key2']}
          >
            <div>content</div>
          </TrackLeafRoutePageView>
        </TrackSearchParamKeys>
      </FakeContextProvider>,
    );

    expect(gtagSpy).toHaveBeenCalledTimes(1);
    expect(gtagSpy).toHaveBeenLastCalledWith(
      'event',
      'page_view',
      expect.objectContaining({
        page_location: 'http://localhost/?key1=val1&key2=val2&key2=val4',
      }),
    );
  });

  it('only track page view once', async () => {
    function TestComponent() {
      return (
        <FakeContextProvider>
          <TrackLeafRoutePageView contentGroup="self">
            <div>content</div>
          </TrackLeafRoutePageView>
        </FakeContextProvider>
      );
    }

    const { rerender } = render(<TestComponent />);

    expect(gtagSpy).toHaveBeenCalledTimes(1);
    expect(gtagSpy).toHaveBeenLastCalledWith(
      'event',
      'page_view',
      expect.anything(),
    );

    rerender(<TestComponent />);
    expect(gtagSpy).toHaveBeenCalledTimes(1);

    rerender(<TestComponent />);
    expect(gtagSpy).toHaveBeenCalledTimes(1);
  });

  it('track page view when URL changes', async () => {
    render(
      <FakeContextProvider
        siblingRoutes={[
          {
            path: '/sibling',
            element: (
              <TrackLeafRoutePageView contentGroup="same-content-group">
                <div>sibling</div>
              </TrackLeafRoutePageView>
            ),
          },
        ]}
      >
        <TrackLeafRoutePageView contentGroup="same-content-group">
          <Link to="/sibling">link to sibling</Link>
        </TrackLeafRoutePageView>
      </FakeContextProvider>,
    );

    expect(gtagSpy).toHaveBeenCalledTimes(1);
    expect(gtagSpy).toHaveBeenLastCalledWith(
      'event',
      'page_view',
      expect.objectContaining({
        page_location: 'http://localhost/',
      }),
    );

    fireEvent.click(screen.getByRole('link'));

    expect(gtagSpy).toHaveBeenCalledTimes(2);
    expect(gtagSpy).toHaveBeenLastCalledWith(
      'event',
      'page_view',
      expect.objectContaining({
        page_location: 'http://localhost/sibling',
      }),
    );
  });

  it('track page view when tracked search param changes', async () => {
    render(
      <FakeContextProvider siblingRoutes={[]}>
        <TrackLeafRoutePageView contentGroup="self" searchParamKeys={['key1']}>
          <Link to="/?key1=val1">self link</Link>
        </TrackLeafRoutePageView>
      </FakeContextProvider>,
    );

    expect(gtagSpy).toHaveBeenCalledTimes(1);
    expect(gtagSpy).toHaveBeenLastCalledWith(
      'event',
      'page_view',
      expect.objectContaining({
        page_location: 'http://localhost/',
      }),
    );

    fireEvent.click(screen.getByRole('link'));

    expect(gtagSpy).toHaveBeenCalledTimes(2);
    expect(gtagSpy).toHaveBeenLastCalledWith(
      'event',
      'page_view',
      expect.objectContaining({
        page_location: 'http://localhost/?key1=val1',
      }),
    );
  });

  it('do not track page view when untracked search param changes', async () => {
    render(
      <FakeContextProvider siblingRoutes={[]}>
        <TrackLeafRoutePageView contentGroup="self" searchParamKeys={['key1']}>
          <Link to="/?key2=val2">self link</Link>
        </TrackLeafRoutePageView>
      </FakeContextProvider>,
    );

    expect(gtagSpy).toHaveBeenCalledTimes(1);
    expect(gtagSpy).toHaveBeenLastCalledWith(
      'event',
      'page_view',
      expect.objectContaining({
        page_location: 'http://localhost/',
      }),
    );

    fireEvent.click(screen.getByRole('link'));

    expect(gtagSpy).toHaveBeenCalledTimes(1);
  });
});
