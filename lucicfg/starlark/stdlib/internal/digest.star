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

"""Functions to calculate hash digests of blocks of data."""

load('@stdlib//internal/strutil.star', 'strutil')


def _size(algo):
  """Returns the number of bytes digest.bytes(algo, ...) would return.

  Args:
    algo: a hashing algorithm to use, e.g. `digest.SHA256`. Required.
  """
  if algo == digest.SHA256:
    return 32
  fail('digest.size: unknown hashing algorithm %r' % (algo,))


def _bytes(algo, blob):
  """Calculates the hash digest of `blob` and returns it as a raw bytes string.

  Args:
    algo: a hashing algorithm to use, e.g. `digest.SHA256`. Required.
    blob: a string with the data to calculate the digest of. Required.

  Returns:
    The digest as a raw bytes string.
  """
  return __native__.hash_digest(algo, blob)


def _hex(algo, blob):
  """Calculates the hash digest of `blob` and returns it as a hex string.

  Args:
    algo: a hashing algorithm to use, e.g. `digest.SHA256`. Required.
    blob: a string with the data to calculate the digest of. Required.

  Returns:
    The digest as a hex-encoded string.
  """
  return strutil.hex_encode(_bytes(algo, blob))


digest = struct(
    size = _size,
    bytes = _bytes,
    hex = _hex,

    # Supported hashing algorithms for `algo` argument.
    SHA256 = 'SHA256',
)
