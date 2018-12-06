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

"""Helpers for parsing and validating service names."""


def _from_host(kind, host, required=False):
  """Validates a service host name and returns a struct with the service info.

  Args:
    kind: a kind of the host, e.g. 'buildbucket' (for error messages).
    host: a service host name, e.g. 'buildbucket.appspot.com'.
    required: if True, fails if host is None or '' instead of returning None.
  """
  if not host:
    if required:
      fail('bad "%s": not set, but it is required' % kind)
    return None

  # Recognize appspot.com, since this is where all LUCI service are.
  app_id = host
  if host.endswith('.appspot.com'):
    app_id = host[:-len('.appspot.com')]

  return struct(
      host = host,
      app_id = app_id,
      cfg_file = '%s.cfg' % app_id,
  )


# API of this module.
service = struct(from_host = _from_host)
