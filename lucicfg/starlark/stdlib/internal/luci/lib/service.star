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

load('@stdlib//internal/validate.star', 'validate')


def _from_host(attr, host):
  """Validates a service host name and returns a struct with the service info.

  Args:
    attr: name of the attribute, e.g. 'buildbucket' (for error messages).
    host: a service host name, e.g. 'buildbucket.appspot.com'.
  """
  host = validate.string(attr, host, required=False)
  if not host:
    return None

  # Recognize appspot.com, since this is where all LUCI services are.
  app_id = host
  if host.endswith('.appspot.com'):
    app_id = host[:-len('.appspot.com')]

  return struct(
      host = host,
      app_id = app_id,
      cfg_file = '%s.cfg' % app_id,
  )


service = struct(from_host = _from_host)
