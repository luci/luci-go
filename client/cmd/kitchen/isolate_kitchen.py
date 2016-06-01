#!/usr/bin/env python
# Copyright 2016 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

import argparse
import contextlib
import os
import shutil
import sys
import subprocess
import tempfile

THIS_DIRECTORY = os.path.dirname(os.path.abspath(__file__))
FNULL = open(os.devnull, 'w')
CONFIGURATIONS = [
  (goos, goarch)
  for goos in ('linux', 'darwin', 'windows')
  for goarch in ('386', 'amd64')
]


def check_isolate_installed():
  """Checks if isolate is installed. If not installs. Returns its path."""
  if subprocess.call(['which', 'isolate'], stdout=FNULL, stderr=FNULL) > 0:
    print >> sys.stderr, 'isolate is not found in $PATH'
    print >> sys.stderr, 'Install it: go install github.com/luci/luci-go/client/cmd/isolate'
    sys.exit(1)


def build_kitchen(workdir, goos, goarch):
  """Builds kitchen to |workdir|/kitchen-|goos|-|goarch|.

  Returns:
    Path to the compiled kitchen.
  """
  output = os.path.join(workdir, 'kitchen-%s-%s' % (goos, goarch))
  if goos == 'windows':
    output += '.exe'

  env = os.environ.copy()
  env.update(GOOS=goos, GARCH=goarch)
  print 'Building kitchen for %s:%s' % (goos, goarch)
  subprocess.check_call(['go', 'build', '-o', output], env=env)
  return output


def parse_args():
  parser = argparse.ArgumentParser()
  parser.add_argument(
    '--isolate-server', '-I',
    default='https://isolateserver.appspot.com',
    help=(
      'Isolate server to use; use special value "fake" to use a fake server'))
  parser.add_argument(
    '--isolated', '-s',
    default='kitchen.isolated',
    help='.isolated file to generate',
  )
  return parser.parse_args()


def isolate(workdir, kitchen_binaries, isolated, isolate_server):
  """Isolates kitchen and returns exit code."""
  print 'Isolating...'
  ENTRY_POINT = 'kitchen.py'
  with open(os.path.join(workdir, ENTRY_POINT), 'w') as f:
    f.write(ENTRY_POINT_SCRIPT)

  isolate_def = {
    'variables': {
      'command': ['python', ENTRY_POINT],
      'files': [ENTRY_POINT] + kitchen_binaries,
    },
  }
  isolate_path = os.path.join(workdir, 'kitchen.isolate')
  with open(isolate_path, 'w') as f:
    f.write('%r' % isolate_def)
  return subprocess.call(
    [
      'isolate', 'archive',
      '-I', isolate_server,
      '-i', isolate_path,
      '-s', isolated,
    ],
    cwd=workdir
  )


@contextlib.contextmanager
def in_this_dir():
  orig_cwd = os.getcwd()
  if orig_cwd == THIS_DIRECTORY:
    yield
    return
  os.chdir(THIS_DIRECTORY)
  try:
    yield
  finally:
    os.chdir(orig_cwd)


def main():
  args = parse_args()
  check_isolate_installed()
  with in_this_dir():
    workdir = tempfile.mkdtemp()
    try:
      files = [
        os.path.relpath(build_kitchen(workdir, goos, goarch), workdir)
        for goos, goarch in CONFIGURATIONS
      ]
      return isolate(workdir, files, args.isolated, args.isolate_server)
    finally:
      shutil.rmtree(workdir, ignore_errors=True)


ENTRY_POINT_SCRIPT = """
import os
import platform
import subprocess
import sys

THIS_DIR = os.path.dirname(os.path.abspath(__file__))


def main():
  goos = platform.system().lower()

  goarch_map = {
    'AMD64': 'amd64',
    'x86_64': 'amd64',
    'i386': '386',
  }
  goarch = goarch_map.get(platform.machine())
  supported = goarch is not None
  if supported:
    kitchen = os.path.join(
      THIS_DIR,
      'kitchen-%s-%s%s' % (goos, goarch, '.exe' if goos == 'windows' else ''))
    supported = os.path.isfile(kitchen)
  if not supported:
    print  >> sys.stderr, 'Unsupported os:arch: %s:%s' % (goos, goarch)
    return 1
  else:
    return subprocess.call([kitchen] + sys.argv[1:])


if __name__ == '__main__':
  sys.exit(main())
"""


if __name__ == '__main__':
  sys.exit(main())
