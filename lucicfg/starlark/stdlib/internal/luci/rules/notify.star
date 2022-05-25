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

"""Defines luci.notify(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys")

def _notify(
        ctx,  # @unused
        *,
        tree_closing_enabled = False):
    """Defines configuration of the LUCI-Notify service for this project.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      tree_closing_enabled: if this is set to False, LUCI-Notify won't close
        trees for this project, just monitor builders and log what actions it
        would have taken.
    """
    key = keys.notify()
    graph.add_node(key, props = {
        "tree_closing_enabled": validate.bool(
            "tree_closing_enabled",
            tree_closing_enabled,
            required = False,
        ),
    })
    graph.add_edge(keys.project(), key)
    return graph.keyset(key)

notify = lucicfg.rule(impl = _notify)
