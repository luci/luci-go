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

"""Defines luci.buildbucket_notification_topic(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/luci/common.star", "keys")
load("@stdlib//internal/luci/proto.star", "common_pb")
load("@stdlib//internal/validate.star", "validate")

# compression str => common_pb.Compression_Enum.
_compression_enum = {
    "ZLIB": common_pb.ZLIB,
    "ZSTD": common_pb.ZSTD,
}

def _buildbucket_notification_topic(
        ctx,  # @unused
        *,
        name,
        compression = "ZLIB"):
    """Define a buildbucket notification topic.

    Buildbucket will publish build notifications (using the luci project scoped
    service account) to this topic every time build status changes. For details,
    see [BuildbucketCfg.builds_notification_topics]

    [BuildbucketCfg.builds_notification_topics]: https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/buildbucket/proto/project_config.proto

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: a full topic name. e.g. 'projects/my-cloud-project/topics/my-topic'. Required.
      compression: specify a compression method. The default is "ZLIB".
    """
    validate.string("name", name, regexp = r"^projects/[^/]+/topics/[^/]+$")
    if compression not in _compression_enum.keys():
        fail('bad "compression" value. It must be in %s' % _compression_enum.keys())

    topic_key = keys.buildbucket_notification_topic(name)
    graph.add_node(topic_key, props = {
        "name": name,
        "compression": _compression_enum[compression],
    })
    graph.add_edge(keys.project(), topic_key)
    return graph.keyset(topic_key)

buildbucket_notification_topic = lucicfg.rule(impl = _buildbucket_notification_topic)
