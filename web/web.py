#!/usr/bin/env python
# Copyright 2016 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

"""Manages web/ resource checkout and building.

This script can be run in one of three modes:
  - As "initialize.py", it will perform resource dependency checkout for
    "luci_deploy" and quit.
  - As "build.py", it will build web apps to a "luci_deploy" directory.
  - As "web.py", it is a user-facing tool to manually build web components.
"""

import argparse
import logging
import os
import pipes
import subprocess
import sys

from distutils.spawn import find_executable

LOGGER = logging.getLogger('web.py')

# The root of the "luci-go" checkout, relative to the current "build.py" file.
_LUCI_GO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# The default build output path.
_WEB_BUILD_PATH = os.path.join(_LUCI_GO_ROOT, 'web')

class Toolchain(object):
  """Web toolchain management."""

  def __init__(self, web_dir, node_exe, npm_exe):
    self._web_dir = web_dir
    self._node_exe = node_exe
    self._npm_exe = npm_exe

  @classmethod
  def initialize(cls, source_root, force=False):
    web_dir = os.path.join(source_root, 'web')

    # Node and NPM must already be installed.
    node_js = [
        find_executable('node'),
        find_executable('npm'),
    ]
    if not all(node_js):
      print """\
  Build requires a "node" and "npm" executables to be installed on your local
  system. Please install Node.js and NPM. Installation instructions can be found
  at:
      https://docs.npmjs.com/getting-started/installing-node
  """
      raise Exception('Unable to locate Node.js installation.')
    tc = cls(web_dir, *node_js)

    # Install NPM deps from "package.json".
    def install_npm_deps():
      tc.npm('install', cwd=web_dir)
    cls._call_if_outdated(
        install_npm_deps,
        os.path.join(web_dir, '.npm.installed'),
        os.path.join(web_dir, 'package.json'),
        force)

    # Install Bower deps from "bower.json".
    def install_bower_deps():
      tc.bower('install', cwd=web_dir)
    cls._call_if_outdated(
        install_bower_deps,
        os.path.join(web_dir, '.bower.installed'),
        os.path.join(web_dir, 'bower.json'),
        force)

    return tc

  @staticmethod
  def _call_if_outdated(fn, manifest_path, defs_path, force):
    """Will call "fn" if the file at "install_path" doesn't match "spec".

    If "fn" completes without raising an exception, the "spec" file will be
    copied to the "installed" path, making subsequent runs of this function a
    no-op until the spec file changes..

    Args:
      fn (callable): The function to call if they don't match.
      manifest_path (str): The path to the installed state file.
      defs_path (str): The path to the source spec file.
      force (bool): If true, call the function regardless.
    """
    with open(defs_path, 'r') as fd:
      spec_data = fd.read()

    if not force and os.path.isfile(manifest_path):
      with open(manifest_path, 'r') as fd:
        current = fd.read()
      if spec_data == current:
        return

    # Either forcing or out of date.
    fn()

    # Update our installed file to match our spec data.
    with open(manifest_path, 'w') as fd:
      fd.write(spec_data)

  @property
  def web_dir(self):
    return self._web_dir

  @property
  def apps_dir(self):
    return os.path.join(self._web_dir, 'apps')

  def _call(self, *args, **kwargs):
    LOGGER.debug('Running command (cwd=%s): %s',
        kwargs.get('cwd', os.getcwd()),
        pipes.quote(' '.join(args)))

    kwargs['stderr'] = subprocess.STDOUT
    subprocess.check_call(args, **kwargs)

  def node(self, *args, **kwargs):
    self._call(self._node_exe, *args, **kwargs)

  def npm(self, *args, **kwargs):
    self._call(self._npm_exe, *args, **kwargs)

  def bower(self, *args, **kwargs):
    exe = os.path.join(self.web_dir, 'node_modules', 'bower', 'bin', 'bower')
    return self.node(exe, *args, **kwargs)

  def gulp(self, *args, **kwargs):
    exe = os.path.join(self.web_dir, 'node_modules', 'gulp', 'bin', 'gulp.js')
    return self.node(exe, *args, **kwargs)


def _subcommand_install():
  # Nothing to do, since toolchain is installed as a precondition to invoking
  # the subcommand!
  return 0


def _subcommand_presubmit(tc):
  # Run Gulp PRESUBMIT.
  tc.gulp('presubmit', cwd=tc.apps_dir)
  return 0


def _subcommand_build(tc, build_dir, apps=None):
  # Build requested apps.
  if not apps:
    # Get all apps with a gulpfile
    apps = [app for app in os.listdir(tc.apps_dir)
            if os.path.isfile(os.path.join(tc.apps_dir, app, 'gulpfile.js'))]

  for app in apps:
    LOGGER.info('Building app [%s] => [%s]', app, build_dir)
    tc.gulp('--out', build_dir,
        cwd=os.path.join(tc.apps_dir, app))
  return 0


def _subcommand_gulp(tc, gulp_args, app=None):
  app_dir = tc.apps_dir
  if app:
    app_dir = os.path.join(app_dir, app)
    if not os.path.isfile(os.path.join(app_dir, 'gulpfile.js')):
      raise ValueError('[%s] is not a valid application' % (app,))
  tc.gulp(*gulp_args, cwd=app_dir)
  return 0


def _main(argv):
  parser = argparse.ArgumentParser()
  parser.add_argument('-v', '--verbose', action='count',
      help='Increase verbosity.')
  parser.add_argument('-i', '--force-install', action='store_true',
      help='Install NPM/Bower files even if they are considered up-to-date.')

  # If we're running the deployment tool version of this script, these arguments
  # are required and positional. Otherwise, they are optional.
  subparser = parser.add_subparsers()

  # Subcommand: install
  subcommand = subparser.add_parser('install',
      help='Install toolchain and exit.')
  subcommand.set_defaults(func=lambda _tc, _args: _subcommand_install())

  # Subcommand: presubmit
  subcommand = subparser.add_parser('presubmit',
      help='Run web presubmit verification.')
  subcommand.set_defaults(func=lambda tc, _args: _subcommand_presubmit(tc))

  # Subcommand: build
  subcommand = subparser.add_parser('build', help='Build web apps.')
  subcommand.set_defaults(func=lambda tc, args:
      _subcommand_build(tc, args.build_dir, args.apps))
  subcommand.add_argument('apps', nargs='*',
      help='Specific apps to build. If none are specified, build all apps.')
  subcommand.add_argument('--build-dir', default=_WEB_BUILD_PATH,
      help='Path to the output build directory. Apps will be written to a '
           '"dist" folder under this path.')

  # Subcommand: gulp
  subcommand = subparser.add_parser('gulp',
      help='Run a global Gulp.js target.')
  subcommand.set_defaults(func=lambda tc, args:
      _subcommand_gulp(tc, args.gulp_args))
  subcommand.add_argument('gulp_args', nargs='*',
      help='Arguments to pass to Gulp.js.')

  # Subcommand: gulp-app
  subcommand = subparser.add_parser('gulp-app',
      help='Run Gulp.js for the specified web app.')
  subcommand.set_defaults(func=lambda tc, args:
      _subcommand_gulp(tc, args.gulp_args, app=args.app))
  subcommand.add_argument('app',
      help='Web app name')
  subcommand.add_argument('gulp_args', nargs='*',
      help='Arguments to pass to Gulp.js.')

  args = parser.parse_args(argv)

  # Set logging level.
  if args.verbose > 0:
    LOGGER.setLevel(logging.DEBUG)

  # Build our generated web content.
  tc = Toolchain.initialize(_LUCI_GO_ROOT, force=args.force_install)
  return args.func(tc, args)


def _main_initialize(argv):
  """Main entry point for "luci_deploy" initialization."""
  # This argument signature is provided by "luci_deploy".
  parser = argparse.ArgumentParser()
  parser.add_argument('source_root',
      help='Path to the source root.')
  parser.add_argument('result_path',
      help='(Unused) result path initialization script argument.')
  args = parser.parse_args(argv)
  Toolchain.initialize(args.source_root)
  return 0


def _main_deploy(argv):
  """Main entry point for "luci_deploy" build."""
  # This argument signature is provided by "luci_deploy".
  parser = argparse.ArgumentParser()
  parser.add_argument('source_root',
      help='Path to the source root.')
  parser.add_argument('build_dir',
      help='Path to the output build directory. Apps will be written to a '
           '"dist" folder under this path.')
  parser.add_argument('apps', nargs='+',
      help='Names of the web apps to build.')
  args = parser.parse_args(argv)
  tc = Toolchain.initialize(args.source_root)
  return _subcommand_build(tc, args.build_dir, args.apps)


def main(argv):
  script_name, args = os.path.basename(argv[0]), argv[1:]
  if script_name == 'initialize.py':
    return _main_initialize(args)
  elif script_name == 'build.py':
    return _main_deploy(args)
  else:
    return _main(args)


if __name__ == '__main__':
  logging.basicConfig(level=logging.INFO)
  sys.exit(main(sys.argv))
