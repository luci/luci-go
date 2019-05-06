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
load('@stdlib//internal/lucicfg.star', 'lucicfg')
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys', 'kinds')
load('@stdlib//internal/luci/lib/acl.star', 'acl', 'aclimpl')
load('@stdlib//internal/luci/lib/cq.star', 'cq', 'cqimpl')

load('@stdlib//internal/luci/rules/cq_tryjob_verifier.star', 'cq_tryjob_verifier')


def _cq_group(
      ctx,
      *,
      name=None,
      watch=None,
      acls=None,
      allow_submit_with_open_deps=None,
      allow_owner_if_submittable=None,
      tree_status_host=None,
      retry_config=None,
      verifiers=None
  ):
  """Defines a set of refs to be watched by the CQ and a set of verifiers to run
  whenever there's a pending approved CL for a ref in the watched set.

  Args:
    name: a name of this CQ group, to reference it in other rules. Doesn't show
        up anywhere in configs or UI. Required.
    watch: either a single cq.refset(...) or a list of cq.refset(...) (one per
        repo), defining what set of refs the CQ should monitor for pending CLs.
        Required.
    acls: list of acl.entry(...) objects with ACLs specific for this CQ group.
        Only `acl.CQ_*` roles are allowed here. By default ACLs are inherited
        from luci.project(...) definition. At least one `acl.CQ_COMMITTER` entry
        should be provided somewhere (either here or in luci.project(...)).
    allow_submit_with_open_deps: controls how a CQ full run behaves when the
        current Gerrit CL has open dependencies (not yet submitted CLs on which
        *this* CL depends). If set to False (default), the CQ will abort a full
        run attempt immediately if open dependencies are detected. If set to
        True, then the CQ will not abort a full run, and upon passing all other
        verifiers, the CQ will attempt to submit the CL regardless of open
        dependencies and whether the CQ verified those open dependencies. In
        turn, if the Gerrit project config allows this, Gerrit will submit all
        dependent CLs first and then this CL.
    allow_owner_if_submittable: allow CL owner to trigger CQ after getting
        `Code-Review` and other approvals regardless of `acl.CQ_COMMITTER` or
        `acl.CQ_DRY_RUNNER` roles. Only `cq.ACTION_*` are allowed here.
        Default is `cq.ACTION_NONE` which grants no additional permissions.
        CL owner is user owning a CL, i.e. its first patchset uploader, not to
        be confused with OWNERS files. **WARNING**: using this option is not
        recommended if you have sticky `Code-Review` label because this allows a
        malicious developer to upload a good looking patchset at first, get code
        review approval, and then upload a bad patchset and CQ it right away.
    tree_status_host: a hostname of the project tree status app (if any). It is
        used by the CQ to check the tree status before committing a CL. If the
        tree is closed, then the CQ will wait until it is reopened.
    retry_config: a new cq.retry_config(...) struct or one of `cq.RETRY_*`
        constants that define how CQ should retry failed builds. See
        [CQ](#cq_doc) for more info. Default is `cq.RETRY_TRANSIENT_FAILURES`.
    verifiers: a list of luci.cq_tryjob_verifier(...) specifying what checks to
        run on a pending CL. See luci.cq_tryjob_verifier(...) for all details.
        As a shortcut, each entry can also either be a dict or a string. A dict
        entry is an alias for `luci.cq_tryjob_verifier(**entry)` and a string
        entry is an alias for `luci.cq_tryjob_verifier(builder = entry)`.
  """
  key = keys.cq_group(validate.string('name', name))

  # Accept cq.refset passed as is (not wrapped in a list). Most CQ configs use
  # a single cq.refset.
  if watch and type(watch) != 'list':
    watch = [watch]
  for w in validate.list('watch', watch, required=True):
    cqimpl.validate_refset('watch', w)

  graph.add_node(key, props = {
      'watch': watch,
      'acls': aclimpl.validate_acls(acls, allowed_roles=[acl.CQ_COMMITTER, acl.CQ_DRY_RUNNER]),
      'allow_submit_with_open_deps': validate.bool(
          'allow_submit_with_open_deps',
          allow_submit_with_open_deps,
          required=False,
      ),
      'allow_owner_if_submittable': validate.int(
          'allow_owner_if_submittable',
          allow_owner_if_submittable,
          default=cq.ACTION_NONE,
          required=False,
      ),
      'tree_status_host': validate.string('tree_status_host', tree_status_host, required=False),
      'retry_config': cqimpl.validate_retry_config(
          'retry_config',
          retry_config,
          default=cq.RETRY_TRANSIENT_FAILURES,
          required=False,
      ),
  })
  graph.add_edge(keys.project(), key)

  # Add all verifiers, possibly instantiating them from dicts or direct builder
  # references (given either as strings or BUILDER_REF keysets).
  for v in validate.list('verifiers', verifiers):
    if type(v) == 'dict':
      v = cq_tryjob_verifier(**v)
    elif type(v) == 'string' or (graph.is_keyset(v) and v.has(kinds.BUILDER_REF)):
      v = cq_tryjob_verifier(builder = v)
    graph.add_edge(key, v.get(kinds.CQ_TRYJOB_VERIFIER))

  return graph.keyset(key)


cq_group = lucicfg.rule(impl = _cq_group)
