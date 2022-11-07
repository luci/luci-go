#!/usr/bin/env python3
# Copyright 2021 The LUCI Authors.
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

from __future__ import absolute_import
from __future__ import print_function

import os
import shutil
import subprocess
import sys


# A list of (original file, what to rename it to prior to a check).
MOD_FILES = [
    ('go.mod', '.go.mod.current'),
    ('go.sum', '.go.sum.current'),
]


def main(args):
  if len(args) != 1:
    print('Want 1 argument: a path to a directory with go.mod')
    return 1
  os.chdir(args[0])

  # Backup existing go.mod and go.sum since we possibly will modify them.
  for old, new in MOD_FILES:
    shutil.copyfile(old, new)

  try:
    # This command modifies go.mod and/or go.sum if they are untidy. There's
    # currently no way to ask it to check their tidiness without overriding
    # them. See https://github.com/golang/go/issues/27005.
    subprocess.check_call(['go', 'mod', 'tidy'])

    # Check the diff, it should be empty.
    for old, new in MOD_FILES:
      mod_diff = diff(new, old)
      if mod_diff:
        print('%s file is stale:' % old)
        print(mod_diff)
        print()
        print('Run "go mod tidy" to update it.')
        return 1

  except subprocess.CalledProcessError as exc:
    print(
        'Failed to call %s, return code %d:\n%s' %
        (exc.cmd, exc.returncode, exc.output))
    return 2

  finally:
    for old, new in MOD_FILES:
      shutil.move(new, old)


def diff(a, b):
  cmd = ['git', 'diff', '--no-index', a, b]
  proc = subprocess.Popen(
      args=cmd,
      stdout=subprocess.PIPE,
      stderr=subprocess.PIPE,
      universal_newlines=True)
  out, err = proc.communicate()
  if proc.returncode and err:
    raise subprocess.CalledProcessError(
        returncode=proc.returncode, cmd=cmd, output=err)
  return out.strip()


if __name__ == '__main__':
  sys.exit(main(sys.argv[1:]))
