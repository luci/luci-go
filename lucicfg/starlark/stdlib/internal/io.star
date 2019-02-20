# Copyright 2019 The LUCI Authors.
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


def _read_file(path):
  """Reads a file and returns its contents as a string.

  Useful for rules that accept large chunks of free form text. By using
  `io.read_file` such text can be kept in a separate file.

  Args:
    path: either a path relative to the currently executing Starlark script, or
        (if starts with `//`) an absolute path within the currently executing
        package. If it is a relative path, it must point somewhere inside the
        current package directory. Required.

  Returns:
    The contents of the file as a string. Fails if there's no such file, it
    can't be read, or it is outside of the current package directory.
  """
  return __native__.read_file(path)


io = struct(read_file = _read_file)
