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

import type { RouteObject } from 'react-router-dom';

/**
 * Escape RegExp special characters. Adapted from [escape-string-regexp][1].
 * We do not use [escape-string-regexp][1] directly because [ES module cannot
 * be imported by Vite's config file for SPA][2].
 *
 * [1]: https://github.com/sindresorhus/escape-string-regexp/blob/main/index.js
 * [2]: https://vitejs.dev/guide/troubleshooting.html#this-package-is-esm-only
 */
function escapeStringRegexp(str: string) {
  return str.replace(/[|\\{}()[\]^$+*?.]/g, '\\$&').replace(/-/g, '\\x2d');
}

/**
 * Construct a regular expression that matches the given routes.
 */
export function regexpsForRoutes(
  routes: readonly RouteObject[],
  basename = '/',
) {
  if (
    !basename.startsWith('/') ||
    (basename.length > 1 && basename.endsWith('/'))
  ) {
    throw new Error(
      `Invalid basename: ${basename}. ` +
        `A properly formatted basename should have a leading slash, but no trailing slash.`,
    );
  }

  // The code below assumes no trailing slash. '/' has a both a leading slash
  // and a tailing slash. Convert it to an empty string to satisfy the
  // constraint.
  if (basename === '/') {
    basename = '';
  }

  return new RegExp(
    // The prefix matches basename.
    `^${escapeStringRegexp(basename)}` +
      // The middle matches a route.
      `(${regexpsForRoutesImpl(routes, '/')})` +
      // The end can be '?', '#', or the end of the string.
      // Also allow an optional trailing '/'.
      `/?(\\?|#|$)`,
    // Always make it case insensitive.
    'i',
  );
}

function regexpsForRoutesImpl(
  routes: readonly RouteObject[],
  prefix: string,
): string | null {
  const routeRegexps = routes
    .map((route) => {
      let routePath = route.path ?? '';

      // Make absolute child relative.
      if (routePath.startsWith('/')) {
        if (!routePath.startsWith(prefix)) {
          throw new Error(
            `Absolute route path "${routePath}" nested under path "${prefix}" is not valid. ` +
              `An absolute child route path must start with the combined path of all its parent routes.`,
          );
        }
        routePath = routePath.slice(prefix.length);
      }

      // The current route doesn't consume a segment. Go directly into the
      // children.
      if (routePath === '') {
        return regexpsForRoutesImpl(route.children || [], prefix);
      }

      // It has no child or the pattern matches every possible child routes. No
      // point digging into the children.
      if (!route.children?.length || routePath.match(/[/^][*][?]?/)) {
        return composableRegexpForPattern(routePath);
      }

      let childPrefix = prefix + routePath;
      // Normalize the child prefix.
      if (!childPrefix.endsWith('/')) {
        childPrefix += '/';
      }

      return (
        // The prefix matches the route path.
        composableRegexpForPattern(routePath) +
        // The suffix is empty, is '/', or matches a child route.
        `(|/|${regexpsForRoutesImpl(route.children, childPrefix)})`
      );
    })
    .filter((routeRegexp) => routeRegexp !== null);

  return routeRegexps.length ? routeRegexps.join('|') : null;
}

/**
 * Computes the regexp for the given react-router path pattern.
 *
 * Note that the regexp
 * 1. requires and consumes the leading slash (unless the pattern can match an
 *    empty string), and
 * 2. does not require nor consumes the trailing slash, and
 * 3. does not enforce the start to be `/^/` and end to be `/[?#]|$/` (i.e
 *    `'prefix/pattern/to/matchandsuffix'` will match the pattern
 *    `'pattern/to/match'`).
 *
 * This is to make the resulting regexp easier to be combined with regexp
 * generated parent/child routes.
 *
 * @param pattern a react-router path pattern.
 * @returns a regexp in string format that can test if a path matches the
 * pattern.
 */
// Do not export since it has some special cases to make it composable.
function composableRegexpForPattern(pattern: string) {
  return pattern
    .split('/')
    .map((seg, i, segList) => {
      if (seg === '') {
        // The pattern is an empty string or we have a trailing '/'. Ignore it.
        if (i === segList.length - 1) {
          return '';
        }

        // We have a leading '/'. Match the start of the string.
        if (i === 0) {
          return '^';
        }
      }

      const isOptional = seg.endsWith('?');
      if (isOptional) {
        seg = seg.slice(0, seg.length - 1);
      }
      let segRegex = '';

      if (seg === '*' && i === segList.length - 1) {
        // The last * is treated as a splat.
        segRegex = '/[^?#]+';
      } else if (seg.startsWith(':')) {
        // `:varName` is a dynamic segment.
        segRegex = '/[^/?#]+';
      } else {
        // Otherwise matches the string as is.
        segRegex = `/${escapeStringRegexp(seg)}`;
      }

      if (isOptional) {
        segRegex = `(${segRegex})?`;
      }
      return segRegex;
    })
    .join('');
}
