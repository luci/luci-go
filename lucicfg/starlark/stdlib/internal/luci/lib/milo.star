# Copyright 2019 The LUCI Authors.
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

load('@stdlib//internal/io.star', 'io')
load('@proto//luci/milo/project_config.proto', milo_pb='milo')


def _load_console_header(path):
  """Reads the console view header definition from a proto file.

  See `Header` message in Milo's [project.proto] for its schema and detailed
  description of all fields.

  As an alternative to loading the header from an external file, you can also
  describe it using a dict with the schema matching `Header` message, passing
  it as `header` to luci.console_view(...):

    luci.console_view(
        ...
        header = {
            'links': [
                {'name': '...', 'links': [...]},
                ...
            ],
        },
        ...
    )

  This is handy for small headers.

  [project.proto]: https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/master/milo/api/config/project.proto

  Args:
    path: either a path relative to the currently executing Starlark script, or
        (if starts with `//`) an absolute path within the currently executing
        package. If it is a relative path, it must point somewhere inside the
        current package directory. Its extension is used to determine the
        expected proto message encoding: `*.json` and `*.jsonpb` imply JSONPB
        encoding, everything else implies text proto encoding. Required.

  Returns:
    An object that can be passed as `header` field to luci.console_view(...).
  """
  return io.read_proto(milo_pb.Header, path)


miloimpl = struct(
    # Note: this is getting exposed publicly as luci.load_console_header.
    load_console_header = _load_console_header,
)
