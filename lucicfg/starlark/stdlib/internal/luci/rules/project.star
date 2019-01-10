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

load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/luci/common.star', 'keys')
load('@stdlib//internal/luci/lib/acl.star', 'aclimpl')
load('@stdlib//internal/luci/lib/service.star', 'service')
load('@stdlib//internal/luci/lib/validate.star', 'validate')


def project(
      name,

      buildbucket=None,
      logdog=None,
      scheduler=None,
      swarming=None,

      acls=None,
  ):
  """Defines a LUCI project.

  There should be exactly one such definition in a single top-level config file.

  Args:
    name: full name of the project.
    buildbucket: hostname of a Buildbucket service to use (if any).
    logdog: hostname of a LogDog service to use (if any).
    scheduler: hostname of a LUCI Scheduler service to use (if any).
    swarming: hostname of a Swarming service to use (if any).
    acls: list of acl.entry objects, will be inherited by all buckets.
  """
  graph.add_node(keys.project(), props = {
      'name': validate.string('name', name),
      'buildbucket': service.from_host('buildbucket', buildbucket),
      'logdog': service.from_host('logdog', logdog),
      'scheduler': service.from_host('scheduler', scheduler),
      'swarming': service.from_host('swarming', swarming),
      'acls': aclimpl.validate_acls(acls, project_level=True),
  })
