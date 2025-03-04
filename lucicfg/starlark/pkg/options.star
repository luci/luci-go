# Copyright 2025 The LUCI Authors.
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

_all = struct(
  fmt = []
)

def _fmt(paths = [], function_args_sort = []):
  """set options for `lucicfg fmt`.

  Args:
    paths: forward-slash delimited path prefixes for which this Rules message
      applies. lucicfg will organize all Rules by path. Rules with duplicate
      path values are not permitted (i.e. you cannot have two Rules with a path
      of "something", nor can you have the path "something" duplicated within
      a single Rules entry).
      When processing files, lucicfg will calculate the file's path as relative to
      this lucicfg package, and will select a single Rules set based on the
      longest matching path prefix. For example, if there are two Rules sets,
      one formatting "a" and another formatting "a/folder", then for the file
      "a/folder/file.star", only the second Rules set would apply.
      If NO Rules set matches the file path, then only default formatting will
      occur (i.e. lucicfg will only apply formatting which is not controlled by
      this Rules message. In particular, this means that formatting will not
      attempt to reorder function call arguments in any way).
      Default is [].
    function_args_sort: a list of arguments allows you to reorder the function
      call sites, based on the name of the arguments.
      If this is set, then all functions will be sorted first by the order of
      its `arg` field, and then alphanumerically. This implies that setting
      this message without setting any `arg` values will sort all function call
      sites alphabetically.
      If this message is completely omitted, no call site function argument
      reordering will occur.
      The sorting only applies to kwarg-style arguments in files we want to
      format.
      Default is [].
  """
  _all.fmt.append(struct(
    paths = paths,
    function_args_sort = function_args_sort,
  ))

# exported
options = struct(
  fmt = _fmt,
)