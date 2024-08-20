# Copyright 2020 The LUCI Authors.
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

"""Defines luci.tree_closer(...) rule."""

load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "notifiable")

def _tree_closer(
        ctx,  # @unused
        *,
        name = None,
        tree_name = None,
        tree_status_host = None,
        failed_step_regexp = None,
        failed_step_regexp_exclude = None,
        template = None,
        notified_by = None):
    """Defines a rule for closing or opening a tree via a tree status app.

    The set of builders that are being observed is defined through `notified_by`
    field here or `notifies` field in luci.builder(...). Whenever a build
    finishes, the builder "notifies" all (but usually none or just one)
    luci.tree_closer(...) objects subscribed to it, so they can decide whether
    to close or open the tree in reaction to the new builder state.

    Note that luci.notifier(...) and luci.tree_closer(...) are both flavors of
    a `luci.notifiable` object, i.e. both are something that "can be notified"
    when a build finishes. They both are valid targets for `notifies` field in
    luci.builder(...). For that reason they share the same namespace, i.e. it is
    not allowed to have a luci.notifier(...) and a luci.tree_closer(...) with
    the same name.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).

      name: name of this tree closer to reference it from other rules. Required.

      tree_name: the identifier of the tree that this rule will open and close.
        For example, 'chromium'. Tree status affects how CQ lands CLs. See
        `tree_status_name` in luci.cq_group(...). Required.
      tree_status_host: **Deprecated**. Please use tree_name instead.
        A hostname of the project tree status app (if any) that this rule will use
        to open and close the tree. Tree status affects how CQ lands CLs. See
        `tree_status_host` in luci.cq_group(...).
      failed_step_regexp: close the tree only on builds which had a failing step
        matching this regex, or list of regexes.
      failed_step_regexp_exclude: close the tree only on builds which don't have
        a failing step matching this regex or list of regexes. May be combined
        with `failed_step_regexp`, in which case it must also have a failed
        step matching that regular expression.
      template: a luci.notifier_template(...) to use to format tree closure
        notifications. If not specified, and a template `default_tree_status`
        is defined in the project somewhere, it is used implicitly by the tree
        closer.

      notified_by: builders to receive status notifications from. This relation
        can also be defined via `notifies` field in luci.builder(...).
    """

    return notifiable.add(
        name = name,
        props = {
            "name": name,
            "kind": "luci.tree_closer",
            "tree_name": validate.string(
                "tree_name",
                tree_name,
                required = False,
            ),
            "tree_status_host": validate.string(
                "tree_status_host",
                tree_status_host,
                required = False,
            ),
            "failed_step_regexp": validate.regex_list(
                "failed_step_regexp",
                failed_step_regexp,
            ),
            "failed_step_regexp_exclude": validate.regex_list(
                "failed_step_regexp_exclude",
                failed_step_regexp_exclude,
            ),
        },
        template = template,
        notified_by = notified_by,
    )

tree_closer = lucicfg.rule(impl = _tree_closer)
