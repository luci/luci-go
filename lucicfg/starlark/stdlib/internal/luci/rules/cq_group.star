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

load('@stdlib//internal/luci/common.star', 'keys')
load('@stdlib//internal/luci/lib/acl.star', 'acl', 'aclimpl')
load('@stdlib//internal/luci/lib/cq.star', 'cqimpl')


# TODO(vadimsh): Add retry_config.
# TODO(vadimsh): Add verifiers.


def cq_group(
      *,
      name=None,
      watch=None,
      acls=None,
      allow_submit_with_open_deps=None,
      tree_status_host=None
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
    tree_status_host: a hostname of the project tree status app (if any). It is
        used by the CQ to check the tree status before committing a CL. If the
        tree is closed, then the CQ will wait until it is reopened.
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
      'acls':  aclimpl.validate_acls(acls, allowed_roles=[acl.CQ_COMMITTER, acl.CQ_DRY_RUNNER]),
      'allow_submit_with_open_deps': validate.bool(
          'allow_submit_with_open_deps',
          allow_submit_with_open_deps,
          required=False,
      ),
      'tree_status_host': validate.string('tree_status_host', tree_status_host, required=False),
  })
  graph.add_edge(keys.project(), key)
  return graph.keyset(key)
