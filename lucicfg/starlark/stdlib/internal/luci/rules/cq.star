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
load('@stdlib//internal/lucicfg.star', 'lucicfg')
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys')


def _cq(
      ctx,
      *,
      submit_max_burst=None,
      submit_burst_delay=None,
      draining_start_time=None,
      status_host=None,
      project_scoped_account=None
  ):
  """Defines optional configuration of the CQ service for this project.

  CQ is a service that monitors Gerrit CLs in a configured set of Gerrit
  projects, launches presubmit jobs (aka tryjobs) whenever a CL is marked as
  ready for CQ, and submits the CL if it passes all checks.

  This optional rule can be used to set global CQ parameters that apply to all
  luci.cq_group(...) defined in the project.

  Args:
    submit_max_burst: maximum number of successful CQ attempts completed by
        submitting corresponding Gerrit CL(s) before waiting
        `submit_burst_delay`. This feature today applies to all attempts
        processed by CQ, across all luci.cq_group(...) instances. Optional, by
        default there's no limit. If used, requires `submit_burst_delay` to be
        set too.
    submit_burst_delay: how long to wait between bursts of submissions of CQ
        attempts. Required if `submit_max_burst` is used.
    draining_start_time: if present, the CQ will refrain from processing any
        CLs, on which CQ was triggered after the specified time. This is an UTC
        RFC3339 string representing the time, e.g. `2017-12-23T15:47:58Z` and
        Z is mandatory.
    status_host: hostname of the CQ status app to push updates to. Optional and
        deprecated.
    project_scoped_account: Whether CQ used a project scoped account (if available)
        to access external systems like Gerrit. This is a security feature helping
	to improve separation between LUCI projects.
  """
  submit_max_burst = validate.int('submit_max_burst', submit_max_burst, required=False)
  submit_burst_delay = validate.duration('submit_burst_delay', submit_burst_delay, required=False)

  if submit_max_burst and not submit_burst_delay:
    fail('bad "submit_burst_delay": required if "submit_max_burst" is used')
  if submit_burst_delay and not submit_max_burst:
    fail('bad "submit_max_burst": required if "submit_burst_delay" is used')

  key = keys.cq()
  graph.add_node(key, props = {
      'submit_max_burst': submit_max_burst,
      'submit_burst_delay': submit_burst_delay,
      'draining_start_time': validate.string('draining_start_time', draining_start_time, required=False),
      'status_host': validate.string('status_host', status_host, required=False),
      'project_scoped_account': validate.bool(
          'project_scoped_account',
	  project_scoped_account,
	  required=False,
      ),
  })
  graph.add_edge(keys.project(), key)

  return graph.keyset(key)


cq = lucicfg.rule(impl = _cq)
