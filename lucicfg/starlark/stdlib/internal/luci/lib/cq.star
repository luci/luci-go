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

"""CQ related supporting structs and functions."""

load('@stdlib//internal/validate.star', 'validate')


# A struct returned by cq.refset(...).
#
# See cq.refset(...) function for all details.
#
# Fields (all private to discourage snooping):
#   __repo: original 'repo' string as is.
#   __refs: a list of regexps for refs in the repo, as is.
#   __repo_key: a tuple with the key to use in sets.
#   __gob_host: name of a gob host (e.g. 'chromium').
#   __gob_proj: name of a project on this host (e.g. 'infra/luci-py').
_refset_ctor = __native__.genstruct('cq.refset')


def _refset(repo=None, *, refs=None):
  """Defines a repository and a set of refs inside to.

  Used in `watch` field of luci.cq_group(...) to specify what refs the CQ should
  be monitoring.

  *** note
  Gerrit ACLs must be configured such that the CQ has read access to these refs,
  otherwise users will be waiting for the CQ to act on their CLs forever.
  ***

  Args:
    repo: URL of a git repository to poll, starting with `https://`. Only
        `*.googlesource.com` repositories are supported currently. Required.
    refs: a list of regular expressions that define the set of refs to monitor
        for CLs, e.g. `refs/heads/.+`. Defaults to `['refs/heads/master']` if
        omitted.

  Returns:
    An opaque struct to be passed to `watch` field of luci.cq_group(...).
  """
  repo = validate.string('repo', repo, regexp=r'https://.+')

  # Deconstruct GoB URL into a (host, repo) tuple. Support only public GoB URLs.

  host, _, proj = repo[len('https://'):].partition('/')

  if not host.endswith('.googlesource.com'):
    fail('bad "repo": only *.googlesource.com repos are supported currently')
  gob = host[:-len('.googlesource.com')]
  if gob.endswith('-review'):
    gob = gob[:-len('-review')]
  if not gob:
    fail('bad "repo": not a valid repository URL')

  if proj.startswith('a/'):
    proj = proj[len('a/'):]
  if proj.endswith('.git'):
    proj = proj[:-len('.git')]
  if not proj:
    fail('bad "repo": not a valid repository URL')

  refs = validate.list('refs', refs)
  for r in refs:
    validate.string('refs', r)

  return _refset_ctor(
      __repo = repo,
      __refs = refs or ['refs/heads/master'],
      __repo_key = ('gob', gob, proj),
      __gob_host = gob,
      __gob_proj = proj,
  )


def _validate_refset(attr, val, default=None, required=True):
  """Validates that `refset` was constructed via cq.refset(...)."""
  return validate.struct(attr, val, _refset_ctor, default=default, required=required)


cq = struct(
    refset = _refset,
)

cqimpl = struct(
    validate_refset = _validate_refset,
)
