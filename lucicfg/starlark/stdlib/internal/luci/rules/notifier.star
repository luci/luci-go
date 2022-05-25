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

"""Defines luci.notifier(...) rule."""

load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "notifiable")
load("@proto//go.chromium.org/luci/buildbucket/proto/common.proto", buildbucket_common_pb = "buildbucket.v2")

def _notifier(
        ctx,  # @unused
        *,
        name = None,

        # Conditions.
        on_occurrence = None,
        on_new_status = None,

        # Step filters.
        failed_step_regexp = None,
        failed_step_regexp_exclude = None,

        # Deprecated conditions.
        on_failure = None,
        on_new_failure = None,
        on_status_change = None,
        on_success = None,

        # Who to notify.
        notify_emails = None,
        notify_rotation_urls = None,
        notify_blamelist = None,

        # Tweaks.
        blamelist_repos_whitelist = None,
        template = None,

        # Relations.
        notified_by = None):
    """Defines a notifier that sends notifications on events from builders.

    A notifier contains a set of conditions specifying what events are
    considered interesting (e.g. a previously green builder has failed), and a
    set of recipients to notify when an interesting event happens. The
    conditions are specified via `on_*` fields, and recipients are specified
    via `notify_*` fields.

    The set of builders that are being observed is defined through `notified_by`
    field here or `notifies` field in luci.builder(...). Whenever a build
    finishes, the builder "notifies" all luci.notifier(...) objects subscribed
    to it, and in turn each notifier filters and forwards this event to
    corresponding recipients.

    Note that luci.notifier(...) and luci.tree_closer(...) are both flavors of
    a `luci.notifiable` object, i.e. both are something that "can be notified"
    when a build finishes. They both are valid targets for `notifies` field in
    luci.builder(...). For that reason they share the same namespace, i.e. it is
    not allowed to have a luci.notifier(...) and a luci.tree_closer(...) with
    the same name.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).

      name: name of this notifier to reference it from other rules. Required.

      on_occurrence: a list specifying which build statuses to notify for.
        Notifies for every build status specified. Valid values are string
        literals `SUCCESS`, `FAILURE`, and `INFRA_FAILURE`. Default is None.
      on_new_status: a list specifying which new build statuses to notify for.
        Notifies for each build status specified unless the previous build
        was the same status. Valid values are string literals `SUCCESS`,
        `FAILURE`, and `INFRA_FAILURE`. Default is None.
      on_failure: Deprecated. Please use `on_new_status` or `on_occurrence`
        instead. If True, notify on each build failure. Ignores transient (aka
        "infra") failures. Default is False.
      on_new_failure: Deprecated. Please use `on_new_status` or `on_occurrence`
        instead. If True, notify on a build failure unless the previous build
        was a failure too. Ignores transient (aka "infra") failures. Default
        is False.
      on_status_change: Deprecated. Please use `on_new_status` or
        `on_occurrence` instead. If True, notify on each change to a build
        status (e.g. a green build becoming red and vice versa). Default is
        False.
      on_success: Deprecated. Please use `on_new_status` or `on_occurrence`
        instead. If True, notify on each build success. Default is False.

      failed_step_regexp: an optional regex or list of regexes, which is matched
        against the names of failed steps. Only build failures containing
        failed steps matching this regex will cause a notification to be sent.
        Mutually exclusive with `on_new_status`.
      failed_step_regexp_exclude: an optional regex or list of regexes, which
        has the same function as `failed_step_regexp`, but negated - this regex
        must *not* match any failed steps for a notification to be sent.
        Mutually exclusive with `on_new_status`.

      notify_emails: an optional list of emails to send notifications to.
      notify_rotation_urls: an optional list of URLs from which to fetch
        rotation members. For each URL, an email will be sent to the currently
        active member of that rotation. The URL must contain a JSON object, with
        a field named 'emails' containing a list of email address strings.
      notify_blamelist: if True, send notifications to everyone in the computed
        blamelist for the build. Works only if the builder has a repository
        associated with it, see `repo` field in luci.builder(...). Default is
        False.

      blamelist_repos_whitelist: an optional list of repository URLs (e.g.
        `https://host/repo`) to restrict the blamelist calculation to. If empty
        (default), only the primary repository associated with the builder is
        considered, see `repo` field in luci.builder(...).
      template: a luci.notifier_template(...) to use to format notification
        emails. If not specified, and a template `default` is defined in the
        project somewhere, it is used implicitly by the notifier.

      notified_by: builders to receive status notifications from. This relation
        can also be defined via `notifies` field in luci.builder(...).
    """
    name = validate.string("name", name)

    on_occurrence = _buildbucket_status_validate(validate.list("on_occurrence", on_occurrence))
    on_new_status = _buildbucket_status_validate(validate.list("on_new_status", on_new_status))

    # deprecated
    on_failure = validate.bool("on_failure", on_failure, required = False)
    on_new_failure = validate.bool("on_new_failure", on_new_failure, required = False)
    on_status_change = validate.bool("on_status_change", on_status_change, required = False)
    on_success = validate.bool("on_success", on_success, required = False)

    failed_step_regexp = validate.regex_list("failed_step_regexp", failed_step_regexp)
    failed_step_regexp_exclude = validate.regex_list("failed_step_regexp_exclude", failed_step_regexp_exclude)

    if (failed_step_regexp or failed_step_regexp_exclude) and (on_new_status or on_new_failure or on_status_change):
        fail("failed step regexes cannot be used in combination with status change predicates")

    if not (on_failure or on_new_failure or on_status_change or on_success or on_occurrence or on_new_status):
        fail("at least one on_... condition is required")

    notify_emails = validate.list("notify_emails", notify_emails)
    for e in notify_emails:
        validate.string("notify_emails", e)

    notify_rotation_urls = validate.list(
        "notify_rotation_urls",
        notify_rotation_urls,
        required = False,
    )
    for r in notify_rotation_urls:
        validate.string("notify_rotation_urls", r)

    notify_blamelist = validate.bool("notify_blamelist", notify_blamelist, required = False)
    blamelist_repos_whitelist = validate.list("blamelist_repos_whitelist", blamelist_repos_whitelist)
    for repo in blamelist_repos_whitelist:
        validate.repo_url("blamelist_repos_whitelist", repo)
    if blamelist_repos_whitelist and not notify_blamelist:
        fail("blamelist_repos_whitelist requires notify_blamelist to be True")

    return notifiable.add(
        name = name,
        props = {
            "name": name,
            "kind": "luci.notifier",
            "on_occurrence": on_occurrence,
            "on_new_status": on_new_status,
            "on_failure": on_failure,
            "on_new_failure": on_new_failure,
            "on_status_change": on_status_change,
            "on_success": on_success,
            "failed_step_regexp": failed_step_regexp,
            "failed_step_regexp_exclude": failed_step_regexp_exclude,
            "notify_emails": notify_emails,
            "notify_rotation_urls": notify_rotation_urls,
            "notify_blamelist": notify_blamelist,
            "blamelist_repos_whitelist": blamelist_repos_whitelist,
        },
        template = template,
        notified_by = notified_by,
    )

def _buildbucket_status_validate(status_list):
    """Validates a list of buildbucket statuses.

    Args:
      status_list: a list of status strings; i.e. "SUCCESS" or "FAILURE"

    Returns:
      A list of the enum identifiers corresponding to the given statuses.
    """
    for status in status_list:
        if not hasattr(buildbucket_common_pb, status):
            fail("%s is not a valid status. Try one of the following %s." %
                 (status, str(dir(buildbucket_common_pb))))

    return [getattr(buildbucket_common_pb, status) for status in status_list]

notifier = lucicfg.rule(impl = _notifier)
