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


def _generator(impl):
  """Registers a callback that is called at the end of the config generation
  stage to modify/append/delete generated configs in an arbitrary way.

  The callback accepts single argument 'ctx' which is a struct with following
  fields:
    'config_set': a dict {config file name -> (str | proto)}.

  The callback is free to modify ctx.config_set in whatever way it wants, e.g.
  by adding new values there or mutating/deleting existing ones.

  Args:
    impl: a callback func(ctx) -> None.
  """
  __native__.add_generator(impl)


# Public API.
core = struct(
    generator = _generator,
)
