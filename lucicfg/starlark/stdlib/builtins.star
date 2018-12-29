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
load('@stdlib//internal/generator.star', _generator='generator')

# Individual LUCI rules.
load('@stdlib//internal/luci/rules/bucket.star', _bucket='bucket')
load('@stdlib//internal/luci/rules/builder.star', _builder='builder')
load('@stdlib//internal/luci/rules/logdog.star', _logdog='logdog')
load('@stdlib//internal/luci/rules/project.star', _project='project')

# LUCI helper modules.
load('@stdlib//internal/luci/lib/acl.star', _acl='acl')

# Register all LUCI config generator callbacks.
load('@stdlib//internal/luci/generators.star', _register='register')
_register()


# Public API.
core = struct(
    generator = _generator,

    bucket = _bucket,
    builder = _builder,
    logdog =  _logdog,
    project = _project,
)
acl = _acl
