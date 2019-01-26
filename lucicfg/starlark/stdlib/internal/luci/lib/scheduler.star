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

"""Scheduler related supporting structs and functions."""

load('@stdlib//internal/validate.star', 'validate')
load('@proto//luci/scheduler/project_config.proto', scheduler_pb='scheduler.config')


def _policy(
      kind,
      max_concurrent_invocations=None,
      max_batch_size=None,
      log_base=None,
  ):
  """Policy for how LUCI Scheduler should handle incoming triggering requests.

  This policy defines when and how LUCI Scheduler should launch new builds in
  response to triggering requests from core.gitiles_poller(...) or from
  EmitTriggers RPC call.

  The following batching strategies are supported:

    * `scheduler.GREEDY_BATCHING_KIND`: use a greedy batching function that
      takes all pending triggering requests (up to `max_batch_size` limit) and
      collapses them into one new build. It doesn't wait for a full batch, nor
      tries to batch evenly.
    * `scheduler.LOGARITHMIC_BATCHING_KIND`: use a logarithmic batching function
      that takes log(N) pending triggers (up to `max_batch_size` limit) and
      collapses them into one new build, where N is the total number of pending
      triggers. The base of the logarithm is defined by `log_base`.

  Args:
    kind: one of `*_BATCHING_KIND` values above. Required.
    max_concurrent_invocations: limit on a number of builds running at the same
        time. If the number of currently running builds launched through LUCI
        Scheduler is more than or equal to this setting, LUCI Scheduler will
        keep queuing up triggering requests, waiting for some running build to
        finish before starting a new one. Default is 1.
    max_batch_size: limit on how many pending triggering requests to "collapse"
        into a new single build. For example, setting this to 1 will make each
        triggering request result in a separate build. When multiple triggering
        request are collapsed into a single build, properties of the most recent
        triggering request are used to derive properties for the build. For
        example, when triggering requests come from a core.gitiles_poller(...),
        only a git revision from the latest triggering request (i.e. the latest
        commit) will end up in the build properties. Default is 1000
        (effectively unlimited).
    log_base: base of the logarithm operation during logarithmic batching. For
        example, setting this to 2, will cause 3 out of 8 pending triggering
        requests to be combined into a single build. Required when using
        `LOGARITHMIC_BATCHING_KIND`, ignored otherwise. Must be larger or equal
        to 1.0001 for numerical stability reasons.

  Returns:
    An opaque triggering policy object.
  """
  policy = scheduler_pb.TriggeringPolicy(
      kind = validate.int('kind', kind),
      max_concurrent_invocations = validate.int(
          'max_concurrent_invocations',
          max_concurrent_invocations,
          min=1,
          required=False,
      ),
      max_batch_size = validate.int(
          'max_batch_size',
          max_batch_size,
          min=1,
          required=False,
      ),
  )
  if policy.kind == scheduler_pb.TriggeringPolicy.LOGARITHMIC_BATCHING:
    policy.log_base = validate.float('log_base', log_base, min=1.0001)
  return policy


def _greedy_batching(max_concurrent_invocations=None, max_batch_size=None):
  """A shortcut for `scheduler.policy(scheduler.GREEDY_BATCHING_KIND, ...).`

  See scheduler.policy(...) for all details.

  Args:
    max_concurrent_invocations: see scheduler.policy(...).
    max_batch_size: see scheduler.policy(...).
  """
  return _policy(
      kind = scheduler_pb.TriggeringPolicy.GREEDY_BATCHING,
      max_concurrent_invocations = max_concurrent_invocations,
      max_batch_size = max_batch_size,
  )


def _logarithmic_batching(
      log_base,
      max_concurrent_invocations=None,
      max_batch_size=None,
  ):
  """A shortcut for `scheduler.policy(scheduler.LOGARITHMIC_BATCHING_KIND, ...)`.

  See scheduler.policy(...) for all details.

  Args:
    log_base: see scheduler.policy(...). Required.
    max_concurrent_invocations: see scheduler.policy(...).
    max_batch_size: see scheduler.policy(...).
  """
  return _policy(
      kind = scheduler_pb.TriggeringPolicy.LOGARITHMIC_BATCHING,
      max_concurrent_invocations = max_concurrent_invocations,
      max_batch_size = max_batch_size,
      log_base = log_base,
  )


def _validate_policy(attr, policy, default=None, required=True):
  """Validates that `policy` was returned by scheduler.policy(...).

  Args:
    attr: field name, for error messages. Required.
    policy: a policy to validate. Required.
    default: a policy to use if `policy` is None. May be None itself.
    required: True if `policy` must be non-None, False to allow None.

  Returns:
    Either `policy` itself or `default` (which may be None).
  """
  return validate.type(attr, policy, scheduler_pb.TriggeringPolicy(), default, required)


scheduler = struct(
    GREEDY_BATCHING_KIND = scheduler_pb.TriggeringPolicy.GREEDY_BATCHING,
    LOGARITHMIC_BATCHING_KIND = scheduler_pb.TriggeringPolicy.LOGARITHMIC_BATCHING,

    policy = _policy,
    greedy_batching = _greedy_batching,
    logarithmic_batching = _logarithmic_batching,
)


schedulerimpl = struct(
    validate_policy = _validate_policy,
)
