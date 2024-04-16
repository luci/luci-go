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

"""Defines luci.cq(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys")

def _cq(
        ctx,  # @unused
        *,
        submit_max_burst = None,
        submit_burst_delay = None,
        draining_start_time = None,
        status_host = None,
        honor_gerrit_linked_accounts = None):
    """Defines optional configuration of the CQ service for this project.

    CQ is a service that monitors Gerrit CLs in a configured set of Gerrit
    projects, and launches tryjobs (which run pre-submit tests etc.) whenever a
    CL is marked as ready for CQ, and submits the CL if it passes all checks.

    **NOTE**: before adding a new luci.cq(...), visit and follow instructions
    at http://go/luci/cv/gerrit-pubsub to ensure that pub/sub integration is
    enabled for all the Gerrit projects.

    This optional rule can be used to set global CQ parameters that apply to all
    luci.cq_group(...) defined in the project.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      submit_max_burst: maximum number of successful CQ attempts completed by
        submitting corresponding Gerrit CL(s) before waiting
        `submit_burst_delay`. This feature today applies to all attempts
        processed by CQ, across all luci.cq_group(...) instances. Optional, by
        default there's no limit. If used, requires `submit_burst_delay` to be
        set too.
      submit_burst_delay: how long to wait between bursts of submissions of CQ
        attempts. Required if `submit_max_burst` is used.
      draining_start_time: **Temporarily not supported, see
        https://crbug.com/1208569. Reach out to LUCI team oncall if you need
        urgent help.**. If present, the CQ will refrain from processing any CLs,
        on which CQ was triggered after the specified time. This is an UTC
        RFC3339 string representing the time, e.g. `2017-12-23T15:47:58Z` and Z
        is mandatory.
      status_host: Optional. Decide whether user has access to the details of
        runs in this Project in LUCI CV UI. Currently, only the following
        hosts are accepted: 1) "chromium-cq-status.appspot.com" where everyone
        can access run details. 2) "internal-cq-status.appspot.com" where only
        Googlers can access run details. Please don't use the public host if
        the Project launches internal builders for public repos. It can leak
        the builder names, which may be confidential.
      honor_gerrit_linked_accounts: Optional. Decide whether LUCI CV should
        consider the primary gerrit accounts and the linked/secondary accounts
        sharing the same permission. That means if the primary account is
        allowed to trigger CQ dry run, the secondary account will also be
        allowed, vice versa.
    """
    submit_max_burst = validate.int("submit_max_burst", submit_max_burst, required = False)
    submit_burst_delay = validate.duration("submit_burst_delay", submit_burst_delay, required = False)

    if submit_max_burst and not submit_burst_delay:
        fail('bad "submit_burst_delay": required if "submit_max_burst" is used')
    if submit_burst_delay and not submit_max_burst:
        fail('bad "submit_max_burst": required if "submit_burst_delay" is used')

    key = keys.cq()
    graph.add_node(key, props = {
        "submit_max_burst": submit_max_burst,
        "submit_burst_delay": submit_burst_delay,
        "draining_start_time": validate.string("draining_start_time", draining_start_time, required = False),
        "status_host": validate.hostname("status_host", status_host, required = False),
        "honor_gerrit_linked_accounts": validate.bool(
            "honor_gerrit_linked_accounts",
            honor_gerrit_linked_accounts,
            default = False,
            required = False,
        ),
    })
    graph.add_edge(keys.project(), key)

    return graph.keyset(key)

cq = lucicfg.rule(impl = _cq)
