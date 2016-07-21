#!/usr/bin/env python
# Copyright 2016 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

import argparse
import logging
import os
import subprocess
import sys


def main(argv):
  parser = argparse.ArgumentParser()
  parser.add_argument('source_root', help='path to the source root')
  parser.add_argument('build_dir', help='path to the build directory')
  parser.add_argument('apps', nargs='+',
      help='names of the web apps to build')
  opts = parser.parse_args(argv)

  # Build our generated web content.
  for app in opts.apps:
    logging.info('Building app [%s] => [%s]', app, opts.build_dir)
    subprocess.check_call(
        ['gulp', '--out', opts.build_dir],
        cwd=os.path.join(opts.source_root, 'web', 'apps', app))

  return 0


if __name__ == '__main__':
  logging.basicConfig(level=logging.DEBUG)
  sys.exit(main(sys.argv[1:]))
