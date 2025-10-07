// Copyright 2023 The LUCI Authors.
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

// Relax length limit to prevent wrapping caused by long paths and make the test
// cases more readable.
/* eslint prettier/prettier: ['error', {singleQuote: true, printWidth: 180}] */

import { RouteObject } from 'react-router';

import { regexpsForRoutes } from './route_utils';

describe('regexpsForRoutes', () => {
  // Test against LUCI UI's route definition to ensure all the cases we use are
  // handled correctly.
  describe('LUCI UI routes', () => {
    // Make a copy here so updating the routes won't break the tests.
    const luciUiRoutes: RouteObject[] = [
      { index: true },
      { path: 'login' },
      { path: 'search' },
      { path: 'builder-search' },
      { path: 'p/:project/test-search' },
      { path: 'p/:project/builders' },
      { path: 'p/:project/g/:group/builders' },
      { path: 'p/:project/builders/:bucket/:builder' },
      { path: 'b/:buildId/*?' },
      {
        path: 'p/:project/builders/:bucket/:builder/:buildNumOrId',
        children: [
          { index: true },
          { path: 'overview' },
          { path: 'test-results' },
          {
            path: 'steps',
            children: [{ path: '*' }],
          },
          { path: 'related-builds' },
          { path: 'timeline' },
          { path: 'blamelist' },
        ],
      },
      {
        path: 'inv/:invId',
        children: [{ index: true }, { path: 'test-results' }, { path: 'invocation-details' }],
      },
      {
        path: 'artifact',
        children: [
          { path: 'text-diff/invocations/:invId/artifacts/:artifactId' },
          { path: 'text-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId' },
          { path: 'image-diff/invocations/:invId/artifacts/:artifactId' },
          { path: 'image-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId' },
          { path: 'raw/invocations/:invId/artifacts/:artifactId' },
          { path: 'raw/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId' },
        ],
      },
      { path: 'test/:projectOrRealm/:testId' },
      {
        path: 'bisection',
        children: [{ index: true }, { path: 'analysis/b/:bbid' }],
      },
      { path: 'swarming', children: [{ path: 'task/:taskId' }] },
    ];

    const regexp = regexpsForRoutes([{ path: 'ui', children: luciUiRoutes }]);

    it('should match defined routes', () => {
      expect(regexp.test('/ui')).toBeTruthy();
      expect(regexp.test('/ui/login')).toBeTruthy();

      expect(regexp.test('/ui/search')).toBeTruthy();
      expect(regexp.test('/ui/builder-search')).toBeTruthy();
      expect(regexp.test('/ui/p/the_project/test-search')).toBeTruthy();

      expect(regexp.test('/ui/p/the_project/builders')).toBeTruthy();
      expect(regexp.test('/ui/p/the_project/g/the_group/builders')).toBeTruthy();

      expect(regexp.test('/ui/p/the_project/builders/the_bucket/the_builder')).toBeTruthy();

      expect(regexp.test('/ui/b/123456')).toBeTruthy();
      expect(regexp.test('/ui/b/123456/overview')).toBeTruthy();
      expect(regexp.test('/ui/b/123456/test-results')).toBeTruthy();

      expect(regexp.test('/ui/p/the_project/builders/the_bucket/the_builder/123456')).toBeTruthy();
      expect(regexp.test('/ui/p/the_project/builders/the_bucket/the_builder/123456/overview')).toBeTruthy();
      expect(regexp.test('/ui/p/the_project/builders/the_bucket/the_builder/123456/test-results')).toBeTruthy();
      expect(regexp.test('/ui/p/the_project/builders/the_bucket/the_builder/123456/steps')).toBeTruthy();
      expect(regexp.test('/ui/p/the_project/builders/the_bucket/the_builder/123456/steps/random/suffix')).toBeTruthy();
      expect(regexp.test('/ui/p/the_project/builders/the_bucket/the_builder/123456/timeline')).toBeTruthy();
      expect(regexp.test('/ui/p/the_project/builders/the_bucket/the_builder/123456/blamelist')).toBeTruthy();

      expect(regexp.test('/ui/inv/inv_id')).toBeTruthy();
      expect(regexp.test('/ui/inv/inv_id/test-results')).toBeTruthy();
      expect(regexp.test('/ui/inv/inv_id/invocation-details')).toBeTruthy();

      expect(regexp.test('/ui/artifact/text-diff/invocations/inv_id/artifacts/artifact_id')).toBeTruthy();
      expect(regexp.test('/ui/artifact/text-diff/invocations/inv_id/tests/test_id/results/result_id/artifacts/artifact_id')).toBeTruthy();
      expect(regexp.test('/ui/artifact/image-diff/invocations/inv_id/artifacts/artifact_id')).toBeTruthy();
      expect(regexp.test('/ui/artifact/image-diff/invocations/inv_id/tests/test_id/results/result_id/artifacts/artifact_id')).toBeTruthy();
      expect(regexp.test('/ui/artifact/raw/invocations/inv_id/artifacts/artifact_id')).toBeTruthy();
      expect(regexp.test('/ui/artifact/raw/invocations/inv_id/tests/test_id/results/result_id/artifacts/artifact_id')).toBeTruthy();

      expect(regexp.test('/ui/test/project_or_realm/test_id')).toBeTruthy();

      expect(regexp.test('/ui/bisection')).toBeTruthy();
      expect(regexp.test('/ui/bisection/analysis/b/123456')).toBeTruthy();
    });

    // React Router allows (and ignores trailing slash).
    it('should match paths with trailing slash', () => {
      expect(regexp.test('/ui/')).toBeTruthy();
      expect(regexp.test('/ui/login/')).toBeTruthy();

      expect(regexp.test('/ui/p/the_project/g/the_group/builders/')).toBeTruthy();

      expect(regexp.test('/ui/b/123456/')).toBeTruthy();
      expect(regexp.test('/ui/b/123456/overview/')).toBeTruthy();
      expect(regexp.test('/ui/b/123456/test-results/')).toBeTruthy();

      expect(regexp.test('/ui/p/the_project/builders/the_bucket/the_builder/123456/')).toBeTruthy();
      expect(regexp.test('/ui/p/the_project/builders/the_bucket/the_builder/123456/overview/')).toBeTruthy();
      expect(regexp.test('/ui/p/the_project/builders/the_bucket/the_builder/123456/steps/random/suffix/')).toBeTruthy();

      expect(regexp.test('/ui/artifact/text-diff/invocations/inv_id/artifacts/artifact_id/')).toBeTruthy();
      expect(regexp.test('/ui/artifact/text-diff/invocations/inv_id/tests/test_id/results/result_id/artifacts/artifact_id/')).toBeTruthy();
    });

    it('should work with search string or hash', () => {
      expect(regexp.test('/ui/login?param')).toBeTruthy();
      expect(regexp.test('/ui/login/?param')).toBeTruthy();

      expect(regexp.test('/ui/login#hash')).toBeTruthy();
      expect(regexp.test('/ui/login/#hash')).toBeTruthy();

      expect(regexp.test('/ui/login?param#hash')).toBeTruthy();
      expect(regexp.test('/ui/login/?param#hash')).toBeTruthy();
    });

    it('should not match invalid paths', () => {
      // Not found.
      expect(regexp.test('/ui/cannot-found-this-route-here')).toBeFalsy();

      // No '/ui' prefix.
      expect(regexp.test('/login')).toBeFalsy();

      // Additional prefix.
      expect(regexp.test('prefix/ui/login')).toBeFalsy();

      // No children defined.
      expect(regexp.test('/ui/search/no-child')).toBeFalsy();

      // No matching child.
      expect(regexp.test('/ui/inv/inv_id/invalid-child')).toBeFalsy();

      // A common prefix but no matching route.
      expect(regexp.test('/ui/p/the_project')).toBeFalsy();
    });
  });

  // Test against some additional routes to cover more cases.
  describe('other routes', () => {
    const otherRoutes = [
      { path: '/prefix/absolute-path' },
      { path: 'conditional-segment/seg?' },
      { path: 'conditional-param/:param?' },
      { path: 'multiple-conditional-segments/seg1?/sub/seg2?' },
      { path: 'multiple-conditional-params/:param1?/sub/:params2?' },
      { path: 'mixed-conditional/:param?/sub/seg?' },
    ];

    it('should reject invalid absolute path', () => {
      expect(() => regexpsForRoutes([{ path: 'other-prefix', children: otherRoutes }])).toThrow();
    });

    it('should support valid absolute path', () => {
      const regexp = regexpsForRoutes([{ path: 'prefix', children: otherRoutes }]);
      expect(regexp.test('/prefix/absolute-path')).toBeTruthy();
      expect(regexp.test('/prefix/prefix/absolute-path')).toBeFalsy();
    });

    it('should support conditional segments or params', () => {
      const regexp = regexpsForRoutes(otherRoutes);
      expect(regexp.test('/conditional-segment')).toBeTruthy();
      expect(regexp.test('/conditional-segment/')).toBeTruthy();
      expect(regexp.test('/conditional-segment?')).toBeTruthy();
      expect(regexp.test('/conditional-segment/seg')).toBeTruthy();
      expect(regexp.test('/conditional-segment/seg/')).toBeTruthy();
      expect(regexp.test('/conditional-segment/seg?')).toBeTruthy();
      expect(regexp.test('/conditional-segment/seg1/seg2')).toBeFalsy();

      expect(regexp.test('/conditional-param')).toBeTruthy();
      expect(regexp.test('/conditional-param/')).toBeTruthy();
      expect(regexp.test('/conditional-param?')).toBeTruthy();
      expect(regexp.test('/conditional-param/param')).toBeTruthy();
      expect(regexp.test('/conditional-param/param/')).toBeTruthy();
      expect(regexp.test('/conditional-param/param?')).toBeTruthy();
      expect(regexp.test('/conditional-param/param1/param2')).toBeFalsy();

      expect(regexp.test('/multiple-conditional-segments/sub')).toBeTruthy();
      expect(regexp.test('/multiple-conditional-segments/seg1/sub')).toBeTruthy();
      expect(regexp.test('/multiple-conditional-segments/sub/seg2')).toBeTruthy();
      expect(regexp.test('/multiple-conditional-segments/seg1/sub/seg2')).toBeTruthy();
      expect(regexp.test('/multiple-conditional-segments/seg1/sub/seg2/seg3')).toBeFalsy();
      expect(regexp.test('/multiple-conditional-segments/seg1/seg2')).toBeFalsy();

      expect(regexp.test('/multiple-conditional-params/sub')).toBeTruthy();
      expect(regexp.test('/multiple-conditional-params/param1/sub')).toBeTruthy();
      expect(regexp.test('/multiple-conditional-params/sub/param2')).toBeTruthy();
      expect(regexp.test('/multiple-conditional-params/param1/sub/param2')).toBeTruthy();
      expect(regexp.test('/multiple-conditional-params/param1/param2/sub/param3')).toBeFalsy();
      expect(regexp.test('/multiple-conditional-params/param1/param2')).toBeFalsy();

      expect(regexp.test('/mixed-conditional/sub')).toBeTruthy();
      expect(regexp.test('/mixed-conditional/param/sub')).toBeTruthy();
      expect(regexp.test('/mixed-conditional/sub/seg')).toBeTruthy();
      expect(regexp.test('/mixed-conditional/param/sub/seg')).toBeTruthy();
      expect(regexp.test('/mixed-conditional/param/param2/sub/seg/seg2')).toBeFalsy();
      expect(regexp.test('/mixed-conditional/')).toBeFalsy();
      expect(regexp.test('/mixed-conditional/param')).toBeFalsy();
      expect(regexp.test('/mixed-conditional/param/seg')).toBeFalsy();
    });

    it('should reject invalid base', () => {
      expect(() => regexpsForRoutes(otherRoutes, 'base')).toThrow();
    });

    it('should work with valid base', () => {
      const regexp = regexpsForRoutes(otherRoutes, '/base');
      expect(regexp.test('/base/prefix/absolute-path')).toBeTruthy();
      expect(regexp.test('/base/prefix/absolute-path')).toBeTruthy();

      expect(regexp.test('/base/mixed-conditional/sub')).toBeTruthy();
      expect(regexp.test('/base/mixed-conditional/param/sub')).toBeTruthy();
      expect(regexp.test('/base/mixed-conditional/sub/seg')).toBeTruthy();
      expect(regexp.test('/base/mixed-conditional/param/sub/seg')).toBeTruthy();

      expect(regexp.test('/prefix/absolute-path')).toBeFalsy();
      expect(regexp.test('/prefix/absolute-path')).toBeFalsy();

      expect(regexp.test('/mixed-conditional/sub')).toBeFalsy();
      expect(regexp.test('/mixed-conditional/param/sub')).toBeFalsy();
      expect(regexp.test('/mixed-conditional/sub/seg')).toBeFalsy();
      expect(regexp.test('/mixed-conditional/param/sub/seg')).toBeFalsy();
    });

    it('is case insensitive', () => {
      const regexp = regexpsForRoutes(otherRoutes, '/base');
      expect(regexp.test('/base/prefix/ABSOLUTE-path')).toBeTruthy();
    });
  });
});
