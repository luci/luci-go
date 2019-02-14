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

load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys')


# TODO(vadimsh): Add build_bug_template support.


_ALLOWED_STORAGE_RE = r'https://storage\.googleapis\.com/.+'


def milo(
      logo=None,
      favicon=None,
  ):
  """Defines optional configuration of the Milo service for this project.

  Milo service is a public user interface for displaying (among other things)
  builds, builders, builder lists (see luci.list_view(...)) and consoles
  (see luci.console_view(...)).

  Args:
    logo: optional https URL to the project logo, must be hosted on
        `storage.googleapis.com`.
    favicon: optional https URL to the project favicon, must be hosted on
        `storage.googleapis.com`.
  """
  key = keys.milo()
  graph.add_node(key, props = {
      'logo': validate.string('logo', logo, regexp=_ALLOWED_STORAGE_RE, required=False),
      'favicon': validate.string('favicon', favicon, regexp=_ALLOWED_STORAGE_RE, required=False),
  })
  graph.add_edge(keys.project(), key)
  return graph.keyset(key)
