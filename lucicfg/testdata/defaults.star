luci.project(
    name = 'project',
    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
    scheduler = 'luci-scheduler.appspot.com',
)

luci.bucket(name = 'bucket')

# Test all supported module-scoped defaults (to catch typos).

luci.builder.defaults.properties.set({'k': 'v'})
luci.builder.defaults.service_account.set('a@example.com')
luci.builder.defaults.caches.set([swarming.cache('path1')])
luci.builder.defaults.execution_timeout.set(12 * time.second)
luci.builder.defaults.dimensions.set({'dim': ['val']})
luci.builder.defaults.priority.set(42)
luci.builder.defaults.swarming_tags.set(['k:v'])
luci.builder.defaults.expiration_timeout.set(16 * time.second)
luci.builder.defaults.triggering_policy.set(scheduler.greedy_batching(
    max_concurrent_invocations=5,
    max_batch_size=10,
))
luci.builder.defaults.build_numbers.set(True)
luci.builder.defaults.experimental.set(True)
luci.builder.defaults.task_template_canary_percentage.set(66)
luci.builder.defaults.repo.set('https://example.com/repo')
luci.builder.defaults.luci_migration_host.set('luci_migration_host')

luci.recipe.defaults.cipd_package.set('recipe/package')
luci.recipe.defaults.cipd_version.set('recipe:version')

luci.cq_group.defaults.acls.set([
    acl.entry(acl.CQ_COMMITTER, groups = ['committers']),
])
luci.cq_group.defaults.allow_submit_with_open_deps.set(True)
luci.cq_group.defaults.allow_owner_if_submittable.set(cq.ACTION_DRY_RUN)
luci.cq_group.defaults.tree_status_host.set('tree-status.example.com')
luci.cq_group.defaults.retry_config.set(cq.RETRY_ALL_FAILURES)
luci.cq_group.defaults.cancel_stale_tryjobs.set(True)

luci.cq_tryjob_verifier.defaults.disable_reuse.set(True)
luci.cq_tryjob_verifier.defaults.experiment_percentage.set(11)
luci.cq_tryjob_verifier.defaults.location_regexp.set(['regexp1'])
luci.cq_tryjob_verifier.defaults.location_regexp_exclude.set(['regexp2'])
luci.cq_tryjob_verifier.defaults.owner_whitelist.set(['owner'])
luci.cq_tryjob_verifier.defaults.equivalent_builder_percentage.set(22)
luci.cq_tryjob_verifier.defaults.equivalent_builder_whitelist.set('group')


luci.builder(
    name = 'builder',
    bucket = 'bucket',
    executable = luci.recipe(name='main/recipe'),
)

luci.cq_group(
    name = 'main cq',
    watch = cq.refset(repo='https://example.googlesource.com/example'),
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ['more-committers']),
        acl.entry(acl.CQ_DRY_RUNNER, groups = ['dry-runners']),
    ],
    verifiers = [
        luci.cq_tryjob_verifier(
            builder = 'builder',
            owner_whitelist = ['more-owners'],
        ),
    ],
)

# Expect configs:
#
#
# === commit-queue.cfg
# config_groups: <
#   gerrit: <
#     url: "https://example-review.googlesource.com"
#     projects: <
#       name: "example"
#       ref_regexp: "refs/heads/master"
#     >
#   >
#   verifiers: <
#     gerrit_cq_ability: <
#       committer_list: "committers"
#       committer_list: "more-committers"
#       dry_run_access_list: "dry-runners"
#       allow_submit_with_open_deps: true
#       allow_owner_if_submittable: DRY_RUN
#     >
#     tree_status: <
#       url: "https://tree-status.example.com"
#     >
#     tryjob: <
#       builders: <
#         name: "project/bucket/builder"
#         disable_reuse: true
#         experiment_percentage: 11
#         location_regexp: "regexp1"
#         location_regexp_exclude: "regexp2"
#         owner_whitelist_group: "more-owners"
#         owner_whitelist_group: "owner"
#       >
#       retry_config: <
#         single_quota: 1
#         global_quota: 2
#         failure_weight: 1
#         transient_failure_weight: 1
#         timeout_weight: 2
#       >
#       cancel_stale_tryjobs: YES
#     >
#   >
# >
# ===
#
# === cr-buildbucket.cfg
# buckets: <
#   name: "bucket"
#   swarming: <
#     builders: <
#       name: "builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       swarming_tags: "k:v"
#       dimensions: "dim:val"
#       recipe: <
#         name: "main/recipe"
#         cipd_package: "recipe/package"
#         cipd_version: "recipe:version"
#         properties_j: "k:\"v\""
#       >
#       priority: 42
#       execution_timeout_secs: 12
#       expiration_secs: 16
#       caches: <
#         name: "path1"
#         path: "path1"
#       >
#       build_numbers: YES
#       service_account: "a@example.com"
#       experimental: YES
#       luci_migration_host: "luci_migration_host"
#       task_template_canary_percentage: <
#         value: 66
#       >
#     >
#   >
# >
# ===
#
# === luci-scheduler.cfg
# job: <
#   id: "builder"
#   acl_sets: "bucket"
#   triggering_policy: <
#     kind: GREEDY_BATCHING
#     max_concurrent_invocations: 5
#     max_batch_size: 10
#   >
#   buildbucket: <
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.bucket"
#     builder: "builder"
#   >
# >
# acl_sets: <
#   name: "bucket"
# >
# ===
#
# === project.cfg
# name: "project"
# ===
