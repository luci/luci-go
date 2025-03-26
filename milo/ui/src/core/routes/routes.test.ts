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

import { regexpsForRoutes } from '@/generic_libs/tools/react_router_utils';

import { routes } from './routes';

describe('routes', () => {
  const regex = regexpsForRoutes([{ path: 'ui', children: routes }]);

  // Undefined routes should always be tested false. We rely on this to ensure
  // unhandled routes are not served by the potentially outdated cache layer,
  // so newly added routes can work correctly even when the cache is not
  // refreshed yet.
  //
  // In particular, we should never define a catch all route in `routes` (we
  // can still use catch all routes when defining a router, just don't include
  // them in the `routes` variable).
  //
  // These tests are more likely to fail due to genuine route changes as new
  // routes are added. Don't test routes that will likely be valid in the
  // future. The ONLY purpose of this test is to ensure random paths are not
  // matched by REAL `routes`. More tests can be found in
  // `react_router_utils.test.tsx`.
  describe('should not match undefined routes', () => {
    it('core', () => {
      // Ensure the regex is not rejecting everything.
      expect(regex.test('/ui/login')).toBeTruthy();

      // Unmatched route.
      expect(regex.test('/ui/cannot-found-this-route-here')).toBeFalsy();

      // Unmatched lab route.
      expect(regex.test('/ui/labs/cannot-found-this-route-here')).toBeFalsy();

      // No '/ui' prefix.
      expect(regex.test('/login')).toBeFalsy();

      // Additional prefix.
      expect(regex.test('prefix/ui/login')).toBeFalsy();
    });

    it('fleet', () => {
      // Ensure the regex is not rejecting everything.
      expect(regex.test('/ui/fleet/labs/devices')).toBeTruthy();

      // /fleet implements its own 404 page so non-existing URL requests should
      // be matched to 404.
      expect(regex.test('/ui/fleet/cannot-found-this-route-here')).toBeTruthy();
    });

    it('swarming', () => {
      // Ensure the regex is not rejecting everything.
      expect(regex.test('/ui/swarming/task/my-task')).toBeTruthy();

      // Unmatched lab route.
      expect(
        regex.test('/ui/swarming/labs/cannot-found-this-route-here'),
      ).toBeFalsy();

      expect(
        regex.test('/ui/swarming/cannot-found-this-route-here'),
      ).toBeFalsy();
    });

    it('tree-status', () => {
      // Ensure the regex is not rejecting everything.
      expect(regex.test('/ui/tree-status/my-tree')).toBeTruthy();

      // Ideally, we should not match everything under this route. This prevents
      // us from adding a new route under this prefix.
      // But this is already the case. There's not much we can do.

      // // Unmatched route.
      // expect(
      //   regex.test('/ui/tree-status/cannot-found-this-route-here'),
      // ).toBeFalsy();

      // // Unmatched lab route.
      // expect(
      //   regex.test('/ui/tree-status/labs/cannot-found-this-route-here'),
      // ).toBeFalsy();
    });

    it('analysis', () => {
      expect(regex.test('/ui/tests/b/12345')).toBeTruthy();
      expect(regex.test('/ui/tests/undefined-page')).toBeFalsy();
    });

    it('auth', () => {
      // Ensure the regex is not rejecting everything.
      expect(regex.test('/ui/auth/groups/test-group')).toBeTruthy();

      // Unmatched route.
      expect(regex.test('/ui/auth/cannot-found-this-route-here')).toBeFalsy();
    });
  });
});
