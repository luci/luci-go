# Copyright 2025 The LUCI Authors.
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

def _googlesource(host, repo, ref, path, minimum_version):
  """Define a source for googlesource.com.

  Args:
    host: a googlesource.com source host. Required.
    repo: a name for the repo in `https://<host>.googlesource.com/<repo>`.
      Required.
    ref: a full git reference (e.g. refs/heads/main) for the repo. The history
      of this reference is used to determine the ordering of all commits.
      Required.
    path: a directory path to the lucicfg package within this source repo.
      Required.
    minimum_version: a full git commit id used as minimum version required for
      the repo. This is a constraint that ensures this version is the ancestry
      of the git commit agreed on. Required.
  Returns:
    A source struct for source fetcher.
  """
  return struct(
    type = "googlesource",

    host = host,
    repo = repo,
    ref = ref,
    path = path,
    minimum_version = minimum_version,

    options = struct(
      local = False,
      submodule = False,
    )
  )

def _read_submodule(path):
  """Reads local submodule and returns the source definition for the path.

  The path must be within a submodule of the current repo for the package.

  Args:
    path: a path to a lucicfg package within a submodule. Required.

  Returns:
    A source struct from the submodule.
  """
  _unused(path)

def _read_local(path):
  """Reads local repo and returns the source definition for the path.

  The path must be within the same repo for the package.

  Args:
    path: a path to a lucicfg package within the repo. Required.

  Returns:
    A source struct from the local path.
  """
  _unused(path)

# exported
source = struct(
  googlesource = _googlesource,
  read_submodule = _read_submodule,
  read_local = _read_local,
)