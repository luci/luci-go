# Copyright 2022 The LUCI Authors.
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

"""Defines luci.bucket_constraints(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/luci/common.star", "keys", "kinds")
load("@stdlib//internal/luci/rules/binding.star", "binding")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")

def _bucket_constraints(
        ctx,  # @unused
        bucket = None,
        pools = None,
        service_accounts = None):
    """Adds constraints to a bucket.

    `service_accounts` added as the bucket's constraints will also be granted
    `role/buildbucket.builderServiceAccount` role.

    Used inline in luci.bucket(...) declarations to provide `pools` and
    `service_accounts` constraints for a bucket. `bucket` argument can be
    omitted in this case:

        luci.bucket(
            name = 'try.shadow',
            shadows ='try',
            ...
            constraints = luci.bucket_constraints(
                pools = ['luci.project.shadow'],
                service_accounts = [`shadow@chops-service-account.com`],
            ),
        )

    luci.builder function implicitly populates the constraints to the
    builder’s bucket. I.e.

        luci.builder(
            'builder',
            bucket = 'ci',
            service_account = 'ci-sa@service-account.com',
        )

    adds 'ci-sa@service-account.com' to bucket ci’s constraints.

    Can also be used to add constraints to a bucket outside of
    the bucket declaration. In particular useful in functions. For example:

        luci.bucket(name = 'ci')
        luci.bucket(name = 'ci.shadow', shadows = 'ci')

        def ci_builder(name, ..., shadow_pool = None):
          luci.builder(name = name, bucket = 'ci', ...)
            if shadow_pool:
              luci.bucket_constraints(
                  bucket = 'ci.shadow',
                  pools = [shadow_pool],
              )

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      bucket: name of the bucket to add the constrains.
      pools: list of allowed swarming pools to add to the bucket's constraints.
      service_accounts: list of allowed service accounts to add to the bucket's
        constraints.
    """
    pools = validate.str_list("pools", pools, required = False)
    service_accounts = validate.str_list("service_accounts", service_accounts, required = False)

    # This will be attached to the realm that has this constraint.
    binding_key = None
    if service_accounts:
        binding_key = binding(
            roles = "role/buildbucket.builderServiceAccount",
            users = service_accounts,
        ).get(kinds.BINDING)

    # Note: name of this node is important only for error messages. It isn't
    # showing up in any generated files and by construction it can't
    # accidentally collide with some other name.
    if bucket == None:
        key = keys.unique(kinds.BUCKET_CONSTRAINTS, "")
    else:
        key = keys.unique(kinds.BUCKET_CONSTRAINTS, keys.bucket(bucket).id)

    graph.add_node(key, props = {
        "pools": pools,
        "service_accounts": service_accounts,
    })
    if bucket != None:
        graph.add_edge(parent = keys.bucket(bucket), child = key)
        if binding_key:
            graph.add_edge(parent = keys.realm(bucket), child = binding_key)

    # This is used to detect bucket_constraints nodes that aren't connected to
    # any bucket. Such orphan nodes aren't allowed.
    graph.add_node(keys.bucket_constraints_root(), idempotent = True)
    graph.add_edge(parent = keys.bucket_constraints_root(), child = key)

    keyset = [key]
    if binding_key:
        keyset.append(binding_key)
    return graph.keyset(*keyset)

bucket_constraints = lucicfg.rule(impl = _bucket_constraints)
