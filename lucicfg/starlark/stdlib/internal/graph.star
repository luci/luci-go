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


def _key(*args):
  """Returns a key with given [(kind, name)] path.

  Args:
    *args: even number of strings: kind1, name1, kind2, name2, ...

  Returns:
    graph.key object representing this path.
  """
  return __native__.graph().key(*args)


def _add_node(key, props=None, trace=None):
  """Adds a node to the graph or fails if such node already exists.

  Also fails if used from a generator callback: at this point the graph is
  frozen and can't be extended.

  Args:
    key: a node key, as returned by graph.key(...).
    props: a dict with node properties, will be frozen.
    trace: a stack trace to associate with the node.

  Returns:
    graph.node object representing the node.
  """
  return __native__.graph().add_node(
      key, props or {}, trace or stacktrace(skip=1))


def _node(key):
  """Returns a node by the key or None if it wasn't added by add_node yet.

  Args:
    key: a node key, as returned by graph.key(...).

  Returns:
    graph.node object representing the node.
  """
  return __native__.graph().node(key)


# Public API of this module.
graph = struct(
    key = _key,
    add_node = _add_node,
    node = _node,
)
