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

"""Defines luci.builder_health_notifier(...) rule."""

load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "bhn")

def _builder_health_notifier(
    ctx, # @unused
    owner_email,
    disable = None,
    additional_emails = None,
    notify_all_healthy = None,
):
    """ Defines a builder health notifier configuration.

    The configuration will be used to aggregate all builders
    belonging to an owner and send out health reports for the
    builders in an email.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).

      owner_email: This is an identifier which is unique within a project.
        Required.

      disabale: Disable is a bool allowing owners to toggle notification settings
	   on or off. Default value is false. Optional.
      additional_emails: Additional_emails is a list of other emails that may want to receive
	   the summary of builders' health. Optional.
      notify_all_health: Notify_all_healthy is a bool which dictates whether to send an email
	   summary stating that all builders are healthy. Default is false. Optional.
    """
    owner_email = validate.string("owner_email", owner_email, required = True)
    disable = validate.bool("disable", disable, required = False)
    additional_emails = validate.list("additional_emails", additional_emails, required = False)
    notify_all_healthy = validate.bool("notify_all_healthy", notify_all_healthy, required = False)

    for e in additional_emails:
        validate.string("additional_email", e)

    return bhn.add(
        owner_email = owner_email,
        props = {
            "owner_email" : owner_email,
            "disable" : disable,
            "additional_emails" : additional_emails,
            "notify_all_healthy" : notify_all_healthy,
        },
    )

builder_health_notifier = lucicfg.rule(impl = _builder_health_notifier)