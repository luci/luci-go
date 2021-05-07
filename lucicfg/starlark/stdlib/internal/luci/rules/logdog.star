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

"""Defines luci.logdog(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys")

def _logdog(ctx, *, gs_bucket = None, cloud_logging_project = None, use_global_logdog_account = False):
    """Defines configuration of the LogDog service for this project.

    Usually required for any non-trivial project.

    It's highly recomended to set False or None on `use_global_logdog_account`.
    If True, you will need to authorize the global Logdog service account access
    to your project, and *other* LUCI projects can export their logs to your
    Cloud Project, if they specify your project in their configs.

    If False, Logdog will use the project-scope account for Cloud Logging
    export, and you can avoid such a problem.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      gs_bucket: base Google Storage archival path, archive logs will be written
        to this bucket/path.
      cloud_logging_project: the name of the Cloud project to export logs.
      use_global_logdog_account: Deprecated. If True, the global logdog service
        account will be used to export logs to the Cloud Logging project.
        If False, the project account will be used.
    """
    key = keys.logdog()

    if use_global_logdog_account and not cloud_logging_project:
        fail("use_global_logdog_account: must be False, " +
             "if cloud_logging_project is unset")

    graph.add_node(key, props = {
        "cloud_logging_project": validate.string(
            "cloud_logging_project",
            cloud_logging_project,
            required = False,
        ),
        "gs_bucket": validate.string(
            "gs_bucket",
            gs_bucket,
            required = False,
        ),
        "use_global_logdog_account": validate.bool(
            "use_global_logdog_account",
            use_global_logdog_account,
            default = False,
            required = False,
        ),
    })
    graph.add_edge(keys.project(), key)
    return graph.keyset(key)

logdog = lucicfg.rule(impl = _logdog)
