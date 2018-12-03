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


def _key(*args):
  """Returns a key with given [(kind, name)] path.

  Args:
    *args: even number of strings: kind1, name1, kind2, name2, ...

  Returns:
    graph.key object representing this path.
  """
  return __native__.graph().key(*args)


# Public API of this module.
graph = struct(
    key = _key,
)
