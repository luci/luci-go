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

# Non-LUCI features.
load('@stdlib//internal/io.star', _io='io')
load('@stdlib//internal/lucicfg.star', _lucicfg='lucicfg')
load('@stdlib//internal/time.star', _time='time')

# Individual LUCI rules.
load('@stdlib//internal/luci/rules/bucket.star', _bucket='bucket')
load('@stdlib//internal/luci/rules/builder.star', _builder='builder')
load('@stdlib//internal/luci/rules/console_view.star', _console_view='console_view')
load('@stdlib//internal/luci/rules/console_view_entry.star', _console_view_entry='console_view_entry')
load('@stdlib//internal/luci/rules/cq.star', _cq='cq')
load('@stdlib//internal/luci/rules/cq_group.star', _cq_group='cq_group')
load('@stdlib//internal/luci/rules/cq_tryjob_verifier.star', _cq_tryjob_verifier='cq_tryjob_verifier')
load('@stdlib//internal/luci/rules/gitiles_poller.star', _gitiles_poller='gitiles_poller')
load('@stdlib//internal/luci/rules/list_view.star', _list_view='list_view')
load('@stdlib//internal/luci/rules/list_view_entry.star', _list_view_entry='list_view_entry')
load('@stdlib//internal/luci/rules/logdog.star', _logdog='logdog')
load('@stdlib//internal/luci/rules/milo.star', _milo='milo')
load('@stdlib//internal/luci/rules/project.star', _project='project')
load('@stdlib//internal/luci/rules/recipe.star', _recipe='recipe')

# LUCI helper modules.
load('@stdlib//internal/luci/lib/acl.star', _acl='acl')
load('@stdlib//internal/luci/lib/scheduler.star', _scheduler='scheduler')
load('@stdlib//internal/luci/lib/swarming.star', _swarming='swarming')
load('@stdlib//internal/luci/lib/cq.star', _cq_helpers='cq')

# Register all LUCI config generator callbacks.
load('@stdlib//internal/luci/generators.star', _register='register')
_register()


# Non-LUCI-specific public API.

io = _io
lucicfg = _lucicfg
time = _time

# LUCI-specific public API. Order of entries matters for documentation.

luci = struct(
    project = _project,
    logdog =  _logdog,
    bucket = _bucket,
    recipe = _recipe,
    builder = _builder,
    gitiles_poller = _gitiles_poller,
    milo = _milo,
    list_view = _list_view,
    list_view_entry = _list_view_entry,
    console_view = _console_view,
    console_view_entry = _console_view_entry,
    cq = _cq,
    cq_group = _cq_group,
    cq_tryjob_verifier = _cq_tryjob_verifier,
)
acl = _acl
scheduler = _scheduler
swarming = _swarming
cq = _cq_helpers
