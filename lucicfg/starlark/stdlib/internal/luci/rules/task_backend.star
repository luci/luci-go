# Copyright 2022 The LUCI Authors.
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

"""TaskBackend integration settings."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys")

def _validate_config(attr, val):
    """Valdiates that the value is in the specified backend config format required.

    attr: field name with this value, for error messages.
    val: a value to validate.
    required: if False, allow 'val' to be None, return 'default' in this case.
    """
    if not val:
        return None
    elif type(val) == "dict":
        return json.encode(validate.str_dict(attr, val))
    elif type(val).startswith("proto.Message"):
        return proto.to_jsonpb(val)
    else:
        fail("bad config: config needs to be a dict with string keys or a proto message.")

def _validate_target(attr, val, required = True):
    """Valdiates that the value is the specified backend target format required.

    e.g. swarming://chromium-swarm

    attr: field name with this value, for error messages.
    val: a value to validate.
    required: if False, allow 'val' to be None, return 'default' in this case.
    """
    val = validate.string(attr = attr, val = val, required = required)

    invalid_keywords = ["http", "rpc"]
    for word in invalid_keywords:
        if word in val:
            fail("bad %r: found invaid word: %s in value: %s", (attr, word, val))
    if len(val.split("://")) != 2:
        fail("bad %r: invalid format for %s" % (attr, val))
    return val

def _task_backend(
        ctx,  # @unused
        name = None,
        target = None,
        config = None):
    """Specifies how Buildbucket should integrate with TaskBackend.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: A local name of the task backend. Required.
      target: URI for this backend, e.g. "swarming://chromium-swarm". Required.
      config: A dict with string keys or a proto message to be interpreted as
        JSON encapsulating configuration for this backend.
    """
    key = keys.task_backend(name)
    graph.add_node(key, props = {
        "target": _validate_target("target", target),
        "config": _validate_config("config", config),
    })
    return graph.keyset(key)

task_backend = lucicfg.rule(impl = _task_backend)
