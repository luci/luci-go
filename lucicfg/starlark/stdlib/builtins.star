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

"""Defines symbols available in the global namespace of any lucicfg script."""

# Non-LUCI features.
load("@stdlib//internal/io.star", _io = "io")
load("@stdlib//internal/lucicfg.star", _lucicfg = "lucicfg")
load("@stdlib//internal/time.star", _time = "time")

# Individual LUCI rules.
load("@stdlib//internal/luci/rules/binding.star", _binding = "binding")
load("@stdlib//internal/luci/rules/bucket.star", _bucket = "bucket")
load("@stdlib//internal/luci/rules/bucket_constraints.star", _bucket_constraints = "bucket_constraints")
load("@stdlib//internal/luci/rules/builder.star", _builder = "builder")
load("@stdlib//internal/luci/rules/buildbucket_notification_topic.star", _buildbucket_notification_topic = "buildbucket_notification_topic")
load("@stdlib//internal/luci/rules/console_view.star", _console_view = "console_view")
load("@stdlib//internal/luci/rules/console_view_entry.star", _console_view_entry = "console_view_entry")
load("@stdlib//internal/luci/rules/cq.star", _cq = "cq")
load("@stdlib//internal/luci/rules/cq_group.star", _cq_group = "cq_group")
load("@stdlib//internal/luci/rules/cq_tryjob_verifier.star", _cq_tryjob_verifier = "cq_tryjob_verifier")
load("@stdlib//internal/luci/rules/custom_role.star", _custom_role = "custom_role")
load("@stdlib//internal/luci/rules/dynamic_builder_template.star", _dynamic_builder_template = "dynamic_builder_template")
load("@stdlib//internal/luci/rules/executable.star", _executable = "executable", _recipe = "recipe")
load("@stdlib//internal/luci/rules/external_console_view.star", _external_console_view = "external_console_view")
load("@stdlib//internal/luci/rules/gitiles_poller.star", _gitiles_poller = "gitiles_poller")
load("@stdlib//internal/luci/rules/list_view.star", _list_view = "list_view")
load("@stdlib//internal/luci/rules/list_view_entry.star", _list_view_entry = "list_view_entry")
load("@stdlib//internal/luci/rules/logdog.star", _logdog = "logdog")
load("@stdlib//internal/luci/rules/milo.star", _milo = "milo")
load("@stdlib//internal/luci/rules/notifier.star", _notifier = "notifier")
load("@stdlib//internal/luci/rules/notifier_template.star", _notifier_template = "notifier_template")
load("@stdlib//internal/luci/rules/notify.star", _notify = "notify")
load("@stdlib//internal/luci/rules/project.star", _project = "project")
load("@stdlib//internal/luci/rules/realm.star", _realm = "realm")
load("@stdlib//internal/luci/rules/task_backend.star", _task_backend = "task_backend")
load("@stdlib//internal/luci/rules/tree_closer.star", _tree_closer = "tree_closer")

# LUCI helper modules.
load("@stdlib//internal/luci/lib/acl.star", _acl = "acl")
load("@stdlib//internal/luci/lib/cq.star", _cq_helpers = "cq")
load("@stdlib//internal/luci/lib/buildbucket.star", _buildbucket = "buildbucket")
load("@stdlib//internal/luci/lib/realms.star", _realms = "realms")
load("@stdlib//internal/luci/lib/resultdb.star", _resultdb = "resultdb")
load("@stdlib//internal/luci/lib/scheduler.star", _scheduler = "scheduler")
load("@stdlib//internal/luci/lib/swarming.star", _swarming = "swarming")

# Register all LUCI config generator callbacks.
load("@stdlib//internal/luci/generators.star", _register = "register")

_register()

# Non-LUCI-specific public API.

io = _io
lucicfg = _lucicfg
time = _time

# LUCI-specific public API. Order of entries matters for documentation.

luci = struct(
    project = _project,
    realm = _realm,
    binding = _binding,
    restrict_attribute = _realms.restrict_attribute,
    custom_role = _custom_role,
    logdog = _logdog,
    bucket = _bucket,
    executable = _executable,
    recipe = _recipe,
    builder = _builder,
    gitiles_poller = _gitiles_poller,
    milo = _milo,
    list_view = _list_view,
    list_view_entry = _list_view_entry,
    console_view = _console_view,
    console_view_entry = _console_view_entry,
    external_console_view = _external_console_view,
    notify = _notify,
    notifier = _notifier,
    tree_closer = _tree_closer,
    notifier_template = _notifier_template,
    cq = _cq,
    cq_group = _cq_group,
    cq_tryjob_verifier = _cq_tryjob_verifier,
    bucket_constraints = _bucket_constraints,
    buildbucket_notification_topic = _buildbucket_notification_topic,
    task_backend = _task_backend,
    dynamic_builder_template = _dynamic_builder_template,
)
acl = _acl
cq = _cq_helpers
buildbucket = _buildbucket
resultdb = _resultdb
scheduler = _scheduler
swarming = _swarming
