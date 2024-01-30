# Copyright 2020 The LUCI Authors.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#      http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
import os
import shutil
import subprocess
import sys


def main(args):
  if len(args) != 1:
    print('Want 1 argument: a path to a directory with go.mod')
    return 1
  os.chdir(args[0])

  # Find version of google.golang.org/genproto in go.mod.
  genproto_version = ""
  with open("go.mod",'r') as f:
    for line in f:
      if "google.golang.org/genproto" in line:
        genproto_version = line.split(" ")[1].strip()
  if not genproto_version:
    print("No approriate google.golang.org/genproto version found in go.sum")
    return 1

  # Find version of google.golang.org/genproto that was used to
  # last update googleapis.
  googleapis_path = os.path.join(
    "common",
    "proto",
    "googleapis",
    "google",
    "GENPROTO_REGEN"
  )
  googleapis_commit_from_googleapis = open(googleapis_path,'r').read().strip()

  if genproto_version != googleapis_commit_from_googleapis:
    print(
      "googleapis proto version is out of sync with the version used by " \
      "google.golang.org/genproto. Please update with the import script at " \
      "common/proto/googleapis/import.sh"
      )
    return 1
  return 0


if __name__ == '__main__':
  sys.exit(main(sys.argv[1:]))
