# Copyright 2018 The LUCI Authors.
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


def _matches(pattern, str):
  """Returns (True, tuple of submatches) with leftmost match of the regular
  expression.

  The submatches tuple has the full match as a first string, followed by
  subexpression matches.

  If there are no matches, returns (False, ()). Fails if the regular expression
  can't be compiled.
  """
  return __native__.re_matches(pattern, str)


# Public API of this module.
re = struct(
    matches = _matches,
)
