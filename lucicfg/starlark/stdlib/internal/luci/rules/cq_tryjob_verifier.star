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

load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys', 'kinds')


def cq_tryjob_verifier(
      builder=None,
      *,
      cq_group=None,
      disable_reuse=None,
      experiment_percentage=None,
      location_regexp=None,
      location_regexp_exclude=None
  ):
  """A verifier in a luci.cq_group(...) that triggers tryjobs for verifying CLs.

  When processing a CL, the CQ examines a list of registered verifiers and
  launches new corresponding builds (called "tryjobs") if it decides this is
  necessary (per the configuration of the verifier and the previous history
  of this CL).

  The CQ automatically retries failed tryjobs (per configured `retry_config` in
  luci.cq_group(...)) and only allows CL to land if each builder has succeeded
  in the latest retry. If a given tryjob result is too old (>1 day) it is
  ignored.

  #### Filtering based on files touched by a CL

  The CQ can examine a set of files touched by the CL and decide to skip this
  verifier. Touching a file means either adding, modifying or removing it.

  This is controlled by `location_regexp` and `location_regexp_exclude` fields:

    * If `location_regexp` is specified and no file in a CL matches any of the
      `location_regexp`, then the CQ will not care about this verifier.
    * If a file in a CL matches any `location_regexp_exclude`, then this file
      won't be considered when matching `location_regexp`.
    * If `location_regexp_exclude` is specified, but `location_regexp` is not,
      `location_regexp` is implied to be `.*`.
    * If neither `location_regexp` nor `location_regexp_exclude` are specified
      (default), the verifier will be used on all CLs.

  The matches are done against the following string:

      <gerrit_url>/<gerrit_project_name>/+/<cl_file_path>

  The file path is relative to the repo root, and it uses Unix `/` directory
  separator.

  The comparison is a full match. The pattern is implicitly anchored with `^`
  and `$`, so there is no need add them.

  This filtering currently cannot be used in any of the following cases:

    * For experimental verifiers (when `experiment_percentage` is non-zero).
    * For verifiers in CQ groups with `allow_submit_with_open_deps = True`.

  Please talk to CQ owners if these restrictions are limiting you.

  ##### Examples

  Enable the verifier for all CLs touching any file in `third_party/WebKit`
  directory of the `chromium/src` repo, but not directory itself:

      luci.cq_tryjob_verifier(
          location_regexp = [
              'https://chromium-review.googlesource.com/chromium/src/[+]/third_party/WebKit/.+',
          ],
      )

  Match a CL which touches at least one file other than `one.txt` inside `all/`
  directory of the Gerrit project `repo`:

      luci.cq_tryjob_verifier(
          location_regexp = ['https://example.com/repo/[+]/.+'],
          location_regexp_exclude = ['https://example.com/repo/[+]/all/one.txt'],
      )

  Match a CL which touches at least one file other than `one.txt` in any
  repository **or** belongs to any other Gerrit server. Note, in this case
  `location_regexp` defaults to `.*`:

      luci.cq_tryjob_verifier(
          location_regexp_exclude = ['https://example.com/repo/[+]/all/one.txt'],
      )

  #### Declaring verifiers

  `cq_tryjob_verifier` is used inline in luci.cq_group(...) declarations to
  provide per-builder verifier parameters. `cq_group` argument can be omitted in
  this case:

      luci.cq_group(
          name = 'Main CQ',
          ...
          verifiers = [
              luci.cq_tryjob_verifier(
                  builder = 'Presubmit',
                  disable_reuse = True,
              ),
              ...
          ],
      )


  It can also be associated with a luci.cq_group(...) outside of
  luci.cq_group(...) declaration. This is in particular useful in functions.
  For example:

      luci.cq_group(name = 'Main CQ')

      def try_builder(name, ...):
        luci.builder(name = name, ...)
        luci.cq_tryjob_verifier(builder = name, cq_group = 'Main CQ')

  Args:
    builder: a builder to launch when verifying a CL, see luci.builder(...).
        As an **experimental** and **unstable** extension, can also be a string
        that starts with `external/`. It indicates that this builder is defined
        elsewhere, and its name should be passed to the CQ as is. In particular,
        to reference a builder in another LUCI project, use
        `external/<project>/<bucket>/<builder>` (filling in concrete values). To
        reference a Buildbot builder use `external/*/<master>/<builder>`, e.g.
        `external/*/master.tryserver.chromium.android/android_tester`. Required.
    cq_group: a CQ group to add the verifier to. Can be omitted if
        `cq_tryjob_verifier` is used inline inside some luci.cq_group(...)
        declaration.
    disable_reuse: if True, a fresh build will be required for each CQ attempt.
        Default is False, meaning the CQ may re-use a successful build triggered
        before the current CQ attempt started. This option is typically used for
        verifiers which run presubmit scripts, which are supposed to be quick to
        run and provide additional OWNERS, lint, etc. checks which are useful to
        run against the latest revision of the CL's target branch.
    experiment_percentage: when this field is present, it marks the verifier as
        experimental. Such verifier is only triggered on a given percentage of
        the CLs and the outcome does not affect the decicion whether a CL can
        land or not. This is typically used to test new builders and estimate
        their capacity requirements.
    location_regexp: a list of regexps that define a set of files whose
        modification trigger this verifier. See the explanation above for all
        details.
    location_regexp_exclude: a list of regexps that define a set of files to
        completely skip when evaluating whether the verifier should be applied
        to a CL or not. See the explanation above for all details.
  """
  # Either one of 'external' or 'builder' will be set, but never both.
  if type(builder) == 'string' and builder.startswith('external/'):
    external = builder[len('external/'):]
    builder = None
  else:
    external = None
    builder = keys.builder_ref(builder, attr='builder')

  location_regexp = validate.list('location_regexp', location_regexp)
  for r in location_regexp:
    validate.string('location_regexp', r)
  location_regexp_exclude = validate.list('location_regexp_exclude', location_regexp_exclude)
  for r in location_regexp_exclude:
    validate.string('location_regexp_exclude', r)

  # Note: CQ does this itself implicitly, but we want configs to be explicit.
  if location_regexp_exclude and not location_regexp:
    location_regexp = ['.*']

  # Note: name of this node is important only for error messages. It isn't
  # showing up in any generated files and by construction it can't accidentally
  # collide with some other name.
  key = keys.unique(kinds.CQ_TRYJOB_VERIFIER, builder.id if builder else external)
  graph.add_node(key, props = {
      'external': external,
      'disable_reuse': validate.bool('disable_reuse', disable_reuse, required=False),
      'experiment_percentage': validate.float(
          'experiment_percentage',
          experiment_percentage,
          min=0.0,
          max=100.0,
          required=False,
      ),
      'location_regexp': location_regexp,
      'location_regexp_exclude': location_regexp_exclude,
  })
  if cq_group:
    graph.add_edge(parent=keys.cq_group(cq_group), child=key)
  if builder:
    graph.add_edge(parent=key, child=builder)

  # This is used to detect cq_tryjob_verifier nodes that aren't connected to any
  # cq_group. Such orphan nodes aren't allowed.
  graph.add_node(keys.cq_verifiers_root(), idempotent=True)
  graph.add_edge(parent=keys.cq_verifiers_root(), child=key)

  return graph.keyset(key)
