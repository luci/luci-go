#!/usr/bin/env python3
# Copyright 2023 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Regenerates .golangci.yaml files based on .go-lintable config and templates.

golangci-lint tool has some usability problems when used in large
"monorepo-like" repositories, which either have many modules or want different
options in different directories:
  https://github.com/golangci/golangci-lint/issues/828
  https://github.com/golangci/golangci-lint/issues/2689

For luci-go, the existing style of sorting imports in most
subdirectories that belong to "some-project" is:

  import (
    "stdlib1"
    "stdlib2"

    "github.com/third-party/pkg1"
    "github.com/third-party/pkg2"

    "go.chromium.org/luci/common/pkg1"
    "go.chromium.org/luci/common/pkg2"

    "go.chromium.org/luci/some-project/pkg1"
    "go.chromium.org/luci/some-project/pkg2"

    _ "blank1"
    _ "blank2"

    . "dot1"
    . "dot2"
  )

Where dot imports are mostly used in tests and mostly related to GoConvey.

This is impossible to express via a single root .golangci.ymal config.
Especially considering that when "some-project-2" imports packages from
"some-project", they fall into"common LUCI imports" category and should be
bundled with the rest of LUCI common imports (this happens when "some-project"
exposes a client package that "some-project-2" is importing).

We could keep the root config and introduce per-project configs. But this end up
being dangerous, since golangci-lint searches for config in the *current
working directory* first, and only if not found, looks at Go package
directories under test. Tricium suggests to run e.g:
    golangci-lint run --fix swarming/server/cfg/...
This already assumes luci-go repo root is the current working directory. This
invocation always picks up the root config, ignoring swarming/.golangci.yaml.

For that reason we remove the root config and instead generate a ton of
per-top-directory configs based on a list in `.go-lintable` file. This list
exists for two reasons:
  * To configure what "templates" to use for generated configs.
  * To simply the linter recipe by telling it where configs are.
"""

import argparse
import configparser
import os
import string
import subprocess
import sys


SCRIPTS_DIR = os.path.dirname(os.path.abspath(__file__))
DEFAULT_ROOT = os.path.dirname(SCRIPTS_DIR)
LINTABLE_LIST = '.go-lintable'


def main():
  parser = argparse.ArgumentParser(description='Generates .golangci.yaml')
  parser.add_argument(
      '--root',
      default=DEFAULT_ROOT,
      type=str,
      help='the root directory')
  parser.add_argument(
      '--check',
      action='store_true',
      help='if set, check existing configs are up-to-date')

  args = parser.parse_args()
  os.chdir(args.root)

  # Discover all checked-in directories with *.go files.
  out = subprocess.run(
      ['git', 'ls-files', '.'], check=True, capture_output=True, text=True)
  go_dirs = sorted(set(
      (os.path.dirname(f) or '.') for f in out.stdout.splitlines()
      if f.endswith('.go')
  ))

  # We don't want a root linter config, since it will override all other
  # configs. It means root *.go files will be unlinted. This is fine, there's
  # only one.
  if '.' in go_dirs:
    go_dirs.remove('.')

  # Read the list of directories that should have a linter config.
  try:
    lint_roots = read_go_lintable(LINTABLE_LIST)
  except (ValueError, OSError) as exc:
    print('Bad %s: %s' % (LINTABLE_LIST, exc), file=sys.stderr)
    return 1

  # For every directory with a Go file, find what linted directories contain it.
  covered_by = {}
  for dirname in go_dirs:
    covered_by[dirname] = [
        root for (root, _) in lint_roots
        if dirname == root or dirname.startswith(root + os.path.sep)
    ]

  # All directories should be covered by at least one linter config.
  uncovered = [
      path for (path, coverage) in covered_by.items()
      if not coverage
  ]
  if uncovered:
    print('These paths are not covered by any linter config:', file=sys.stderr)
    for path in uncovered:
      print('  %s' % path, file=sys.stderr)
    print('Modify %s to cover them.' % LINTABLE_LIST, file=sys.stderr)
    return 1

  # All directories should be covered by at most one config. Otherwise
  # golangci-lint may get confused and pick wrong config.
  dups = False
  for (path, coverage) in covered_by.items():
    if coverage and len(coverage) != 1:
      dups = True
      print(
          'Path %s is covered by several linter configs: %s' % (path, coverage),
          file=sys.stderr)
  if dups:
    print(
        'Modify %s to make sure there are no intersections.' % LINTABLE_LIST,
        file=sys.stderr)
    return 1

  # Actually generate all configs in requested lint_roots.
  templates = {}
  configs = {}
  for (root, template) in lint_roots:
    body = templates.get(template)
    if body is None:
      try:
        body = read_template(template)
      except (ValueError, OSError) as exc:
        print('Bad template %s: %s' % (template, exc), file=sys.stderr)
        return 1
      templates[template] = body
    configs[os.path.join(root, '.golangci.yaml')] = body.substitute(root=root)

  # Check everything is already in place if running as a presubmit check.
  if args.check:
    errors = False
    for path, body in configs.items():
      try:
        with open(path, 'r') as f:
          if f.read() != body:
            print('Out-of-date linter config: %s' % path, file=sys.stderr)
            errors = True
      except FileNotFoundError:
        print('Missing linter config: %s' % path, file=sys.stderr)
        errors = True
    if errors:
      print('\nTo regenerate, run:', file=sys.stderr)
      if args.root == DEFAULT_ROOT:
        print('  %s' % __file__, file=sys.stderr)
      else:
        print('  %s --root %s' % (__file__, args.root), file=sys.stderr)
      return 1
    return 0

  for path, body in configs.items():
    with open(path, 'w') as f:
      f.write(body)
  return 0


def read_go_lintable(path):
  """Produces a list of pairs `[(path, template)]`."""
  config = configparser.ConfigParser()
  config.read(path)
  pairs = []
  for section in config:
    if section == 'DEFAULT':
      continue
    template = config[section]['template']
    for path in config[section]['paths'].split():
      path = path.strip()
      if path:
        pairs.append((path, template))
  return pairs


def read_template(name):
  with open(os.path.join(SCRIPTS_DIR, 'golangci', name)) as f:
    return string.Template(f.read())


if __name__ == '__main__':
  sys.exit(main())
