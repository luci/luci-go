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

def declare(name):
  """Sets the import name for this lucicfg package.

  The name must start with @ - all load statements referring to this package as
  remote code will use this name to refer to .star files within this
  package.

  For repos containing recipes, this should be the same value as the recipe
  repo-name for consistency, though to avoid recipe<->lucicfg entanglement,
  this is not a requirement.

  This statement is required exactly once.

  The names `@stdlib`, `@__main__` and `@proto` are reserved.

  Args:
    name: the name for this package. Required.
  """

  _unused(name)

def require_lucicfg(ver):
  """Set the minimal lucicfg version required by this package.

  This statement is required exactly once.

  Args:
    ver: the minimal lucicfg version required. Required.
  """
  _unused(ver)

def depend(name, src):
  """Require package dependency for the .star files in this package.

  Source information need to be provided and a pin file will be calculated
  based on its constrains.

  Name must match the name declared by the packacke.

  Args:
    name: the name of the depended package. Required.
    src: a source definition for fetcher to make package source available. Required.
  """
  _unused(name, src)

def _unused(*args):  # @unused
    """Used exclusively to shut up `unused-variable` lint.

    DocTags:
      Hidden.
    """

load("@pkg//source.star", _source = "source")
load("@pkg//options.star", _options = "options")

source = _source
options = _options
