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
import sys
import re


goLineRe = re.compile(r'^go (.*)$')


def main(args):
  if len(args) != 1:
    print('Want 1 argument: a path to a directory with go.mod')
    return 1
  os.chdir(args[0])

  # Read build/GO_VERSION
  build_go_version = open('build/GO_VERSION', encoding='utf8').read().strip()

  # Find the 'go <version>' line.
  with open('go.mod', encoding='utf8') as gomod:
    for line in gomod:
      if m := goLineRe.match(line):
        go_mod_version = m.group(1).strip()
        break
    else:
      print('Failed to find "go" line in go.mod')
      return 2

  if build_go_version != go_mod_version:
    print('build/GO_VERSION and go.mod do not agree')
    print(f'{build_go_version!r} vs {go_mod_version!r}')
    return 1

  return 0


if __name__ == '__main__':
  sys.exit(main(sys.argv[1:]))

