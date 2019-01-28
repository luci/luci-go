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

"""Utilities for working with strings."""


def _expand_int_set(s):
  """Expands string with sets (e.g. `a{1..3}b`) into list of strings, e.g.
  `['a1b', 'a2b', 'a3b']`.

  The incoming string should have no more than one `{...}` section. If it's
  absent, the function returns the list that contains one item: the original
  string.

  The set is given as comma-separated list of terms. Each term is either
  a single non-negative integer (e.g. `9`) or a range (e.g. `1..5`). Both ends
  of the range are inclusive. Ranges where the left hand side is larger than the
  right hand side are not allowed. All elements should be listed in the strictly
  increasing order (e.g. `1,2,5..10` is fine, but `5..10,1,2` is not). Spaces
  are not allowed.

  The output integers are padded with zeros to match the width of corresponding
  terms. For ranges this works only if both sides have same width. For example,
  `01,002,03..04` will expand into `01, 002, 03, 04`.

  Use `{{` and `}}` to escape `{` and `}` respectively.

  Args:
    s: a string with the set to expand. Required.

  Returns:
    A list of strings representing the expanded set.
  """
  # Implementation is in Go, since it is simpler there, considering Starlark
  # strings aren't even iterable.
  return __native__.expand_int_set(s)


strutil = struct(
    expand_int_set = _expand_int_set,
)
