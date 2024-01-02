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

load("@stdlib//internal/luci/proto.star", "scheduler_pb")
load("@stdlib//internal/time.star", "time")
load("@stdlib//internal/validate.star", "validate")
load("@proto//google/protobuf/duration.proto", duration_pb = "google.protobuf")

def _policy(
        *,
        kind,
        max_concurrent_invocations = None,
        max_batch_size = None,
        log_base = None,
        pending_timeout = None):
    """Policy for how LUCI Scheduler should handle incoming triggering requests.

    This policy defines when and how LUCI Scheduler should launch new builds in
    response to triggering requests from luci.gitiles_poller(...) or from
    EmitTriggers RPC call.

    The following batching strategies are supported:

      * `scheduler.GREEDY_BATCHING_KIND`: use a greedy batching function that
        takes all pending triggering requests (up to `max_batch_size` limit) and
        collapses them into one new build. It doesn't wait for a full batch, nor
        tries to batch evenly.
      * `scheduler.LOGARITHMIC_BATCHING_KIND`: use a logarithmic batching
        function that takes `floor(log(base,N))` pending triggers (at least 1
        and up to `max_batch_size` limit) and collapses them into one new build,
        where N is the total number of pending triggers. The base of the
        logarithm is defined by `log_base`.
      * `scheduler.NEWEST_FIRST`: use a function that prioritizes the most recent
         pending triggering requests. Triggers stay pending until they either
        become the most recent pending triggering request or expire. The timeout
        for pending triggers is specified by `pending_timeout`.

    Args:
      kind: one of `*_BATCHING_KIND` values above. Required.
      max_concurrent_invocations: limit on a number of builds running at the
        same time. If the number of currently running builds launched through
        LUCI Scheduler is more than or equal to this setting, LUCI Scheduler
        will keep queuing up triggering requests, waiting for some running build
        to finish before starting a new one. Default is 1.
      max_batch_size: limit on how many pending triggering requests to
        "collapse" into a new single build. For example, setting this to 1 will
        make each triggering request result in a separate build. When multiple
        triggering request are collapsed into a single build, properties of the
        most recent triggering request are used to derive properties for the
        build. For example, when triggering requests come from a
        luci.gitiles_poller(...), only a git revision from the latest triggering
        request (i.e. the latest commit) will end up in the build properties.
        This value is ignored by NEWEST_FIRST, since batching isn't well-defined
        in that policy kind. Default is 1000 (effectively unlimited).
      log_base: base of the logarithm operation during logarithmic batching. For
        example, setting this to 2, will cause 3 out of 8 pending triggering
        requests to be combined into a single build. Required when using
        `LOGARITHMIC_BATCHING_KIND`, ignored otherwise. Must be larger or equal
        to 1.0001 for numerical stability reasons.
      pending_timeout: how long until a pending trigger is discarded. For example,
        setting this to 1 day will cause triggers that stay pending
        for at least 1 day to be removed from consideration. This value is ignored
        by policy kinds other than NEWEST_FIRST, which can starve old triggers and
        cause the pending triggers list to grow without bound.
        Default is 7 days.

    Returns:
      An opaque triggering policy object.
    """
    policy = scheduler_pb.TriggeringPolicy(
        kind = validate.int("kind", kind),
        max_concurrent_invocations = validate.int(
            "max_concurrent_invocations",
            max_concurrent_invocations,
            min = 1,
            required = False,
        ),
        max_batch_size = validate.int(
            "max_batch_size",
            max_batch_size,
            min = 1,
            required = False,
        ),
        pending_timeout = _duration_as_proto(validate.duration(
            "pending_timeout",
            pending_timeout,
            required = False,
        )),
    )
    if policy.kind == scheduler_pb.TriggeringPolicy.LOGARITHMIC_BATCHING:
        policy.log_base = validate.float("log_base", log_base, min = 1.0001)
    if policy.kind != scheduler_pb.TriggeringPolicy.NEWEST_FIRST and pending_timeout:
        fail("bad pending_timeout: must not be set for non-NEWEST_FIRST policies because it has no effect")
    return policy

def _duration_as_proto(duration):
    """Converts a duration to a duration proto.

    If None is passed, None is returned.

    Args:
      duration: the duration to be converted. Optional.

    Returns:
      A google.protobuf.Duration proto representing the same duration.
    """
    if duration == None:
        return None
    return duration_pb.Duration(
        seconds = duration // time.second,
        nanos = (duration % time.second) * 1000000,
    )

def _greedy_batching(
        *,
        max_concurrent_invocations = None,
        max_batch_size = None):
    """Shortcut for `scheduler.policy(scheduler.GREEDY_BATCHING_KIND, ...).`

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
        *,
        log_base,
        max_concurrent_invocations = None,
        max_batch_size = None):
    """Shortcut for `scheduler.policy(scheduler.LOGARITHMIC_BATCHING_KIND, ...)`.

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

def _newest_first(
        *,
        max_concurrent_invocations = None,
        pending_timeout = None):
    """Shortcut for `scheduler.policy(scheduler.NEWEST_FIRST_KIND, ...)`.

    See scheduler.policy(...) for all details.

    Args:
      max_concurrent_invocations: see scheduler.policy(...).
      pending_timeout: see scheduler.policy(...).
    """
    return _policy(
        kind = scheduler_pb.TriggeringPolicy.NEWEST_FIRST,
        max_concurrent_invocations = max_concurrent_invocations,
        pending_timeout = pending_timeout,
    )

def _validate_policy(attr, policy, *, default = None, required = True):
    """Validates that `policy` was returned by scheduler.policy(...).

    Args:
      attr: field name, for error messages. Required.
      policy: a policy to validate. Required.
      default: a policy to use if `policy` is None. May be None itself.
      required: True if `policy` must be non-None, False to allow None.

    Returns:
      Either `policy` itself or `default` (which may be None).
    """
    return validate.type(
        attr,
        policy,
        scheduler_pb.TriggeringPolicy(),
        default = default,
        required = required,
    )

scheduler = struct(
    GREEDY_BATCHING_KIND = scheduler_pb.TriggeringPolicy.GREEDY_BATCHING,
    LOGARITHMIC_BATCHING_KIND = scheduler_pb.TriggeringPolicy.LOGARITHMIC_BATCHING,
    NEWEST_FIRST_KIND = scheduler_pb.TriggeringPolicy.NEWEST_FIRST,
    policy = _policy,
    greedy_batching = _greedy_batching,
    logarithmic_batching = _logarithmic_batching,
    newest_first = _newest_first,
)

schedulerimpl = struct(
    validate_policy = _validate_policy,
)
