meta.config(config_dir = '.output')
meta.config(tracked_files = ['*.cfg'])
meta.config(fail_on_warnings = True)

core.project(
    name = 'infra',

    buildbucket = 'cr-buildbucket.appspot.com',
    logdog = 'luci-logdog.appspot.com',
    scheduler = 'luci-scheduler.appspot.com',
    swarming = 'chromium-swarm.appspot.com',

    acls = [
        acl.entry(
            roles = [
                acl.PROJECT_CONFIGS_READER,
                acl.LOGDOG_READER,
                acl.BUILDBUCKET_READER,
                acl.SCHEDULER_READER,
            ],
            groups = ['all'],
        ),
        acl.entry(
            roles = [
                acl.BUILDBUCKET_OWNER,
                acl.SCHEDULER_OWNER,
            ],
            groups = ['admins'],
        ),
    ],
)

core.logdog(gs_bucket = 'chromium-luci-logdog')


# Recipes.

core.recipe(
    name = 'main/recipe',
    cipd_package = 'recipe/bundles/main',
)


# CI bucket.

core.bucket(
    name = 'ci',

    # Allow developers to force-launch CI builds through Scheduler, but not
    # directly through Buildbucket. The direct access to Buildbucket allows to
    # override almost all aspects of the builds (e.g. what recipe is used),
    # and Buildbucket totally ignores any concurrency limitations set in the
    # LUCI Scheduler configs. This makes direct Buildbucket access to CI buckets
    # dangerous. They usually have very small pool of machines, and these
    # machines are assumed to be running only "approved" code (being post-submit
    # builders).
    acls = [
        acl.entry(acl.SCHEDULER_TRIGGERER, groups = ['devs']),
    ],
)

core.gitiles_poller(
    name = 'master-poller',
    bucket = 'ci',
    repo = 'https://noop.com',
    refs = ['refs/heads/master', 'refs/tags/blah'],
    refs_regexps = ['refs/branch-heads/\d+\.\d+'],
    schedule = 'with 10s interval',
)

core.builder(
    name = 'linux ci builder',
    bucket = 'ci',
    recipe = 'main/recipe',

    triggered_by = ['master-poller'],
    triggers = ['ci/generically named builder'],

    properties = {
        'prop1': 'val1',
        'prop2': ['val2', 123],
    },
    service_account = 'builder@example.com',

    caches = [
        swarming.cache('path1'),
        swarming.cache('path2', name='name2'),
        swarming.cache('path3', name='name3', wait_for_warm_cache=10*time.minute),
    ],
    execution_timeout = 3 * time.hour,

    dimensions = {
        'os': 'Linux',
        'builder': 'linux ci builder',  # no auto_builder_dimension
        'prefer_if_available': [
            swarming.dimension('first-choice', expiration=5*time.minute),
            swarming.dimension('fallback'),
        ],
    },
    priority = 80,
    swarming_tags = ['tag1:val1', 'tag2:val2'],
    expiration_timeout = time.hour,
    build_numbers = True,

    triggering_policy = scheduler.greedy_batching(
        max_concurrent_invocations=5,
        max_batch_size=10,
    )
)

core.builder(
    name = 'generically named builder',
    bucket = 'ci',
    recipe = 'main/recipe',

    triggered_by = ['master-poller'],
)

core.builder(
    name = 'cron builder',
    bucket = 'ci',
    recipe = 'main/recipe',
    schedule = '0 6 * * *',
)


# Try bucket.

core.bucket(
    name = 'try',

    # Allow developers to launch try jobs directly with whatever parameters
    # they want. Try bucket is basically a free build farm for all developers.
    acls = [
        acl.entry(acl.BUILDBUCKET_TRIGGERER, groups='devs'),
    ],
)

core.builder(
    name = 'linux try builder',
    bucket = 'try',
    recipe = 'main/recipe',
)

core.builder(
    name = 'generically named builder',
    bucket = 'try',
    recipe = 'main/recipe',
)


# Expect configs:
#
# === cr-buildbucket.cfg
# buckets: <
#   name: "ci"
#   acl_sets: "ci"
#   swarming: <
#     builders: <
#       name: "cron builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#         name: "main/recipe"
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/master"
#       >
#     >
#     builders: <
#       name: "generically named builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#         name: "main/recipe"
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/master"
#       >
#     >
#     builders: <
#       name: "linux ci builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       swarming_tags: "tag1:val1"
#       swarming_tags: "tag2:val2"
#       dimensions: "builder:linux ci builder"
#       dimensions: "os:Linux"
#       dimensions: "300:prefer_if_available:first-choice"
#       dimensions: "prefer_if_available:fallback"
#       recipe: <
#         name: "main/recipe"
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/master"
#         properties_j: "prop1:\"val1\""
#         properties_j: "prop2:[\"val2\",123]"
#       >
#       priority: 80
#       execution_timeout_secs: 10800
#       expiration_secs: 3600
#       caches: <
#         name: "name2"
#         path: "path2"
#       >
#       caches: <
#         name: "name3"
#         path: "path3"
#         wait_for_warm_cache_secs: 600
#       >
#       caches: <
#         name: "path1"
#         path: "path1"
#       >
#       build_numbers: YES
#       service_account: "builder@example.com"
#     >
#   >
# >
# buckets: <
#   name: "try"
#   acl_sets: "try"
#   swarming: <
#     builders: <
#       name: "generically named builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#         name: "main/recipe"
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/master"
#       >
#     >
#     builders: <
#       name: "linux try builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#         name: "main/recipe"
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/master"
#       >
#     >
#   >
# >
# acl_sets: <
#   name: "ci"
#   acls: <
#     role: WRITER
#     group: "admins"
#   >
#   acls: <
#     group: "all"
#   >
# >
# acl_sets: <
#   name: "try"
#   acls: <
#     role: WRITER
#     group: "admins"
#   >
#   acls: <
#     group: "all"
#   >
#   acls: <
#     role: SCHEDULER
#     group: "devs"
#   >
# >
# ===
#
# === luci-logdog.cfg
# reader_auth_groups: "all"
# archive_gs_bucket: "chromium-luci-logdog"
# ===
#
# === luci-scheduler.cfg
# job: <
#   id: "cron builder"
#   schedule: "0 6 * * *"
#   acl_sets: "ci"
#   buildbucket: <
#     server: "cr-buildbucket.appspot.com"
#     bucket: "ci"
#     builder: "cron builder"
#   >
# >
# job: <
#   id: "generically named builder"
#   acls: <
#     role: TRIGGERER
#     granted_to: "builder@example.com"
#   >
#   acl_sets: "ci"
#   buildbucket: <
#     server: "cr-buildbucket.appspot.com"
#     bucket: "ci"
#     builder: "generically named builder"
#   >
# >
# job: <
#   id: "linux ci builder"
#   acl_sets: "ci"
#   triggering_policy: <
#     kind: GREEDY_BATCHING
#     max_concurrent_invocations: 5
#     max_batch_size: 10
#   >
#   buildbucket: <
#     server: "cr-buildbucket.appspot.com"
#     bucket: "ci"
#     builder: "linux ci builder"
#   >
# >
# trigger: <
#   id: "master-poller"
#   schedule: "with 10s interval"
#   acl_sets: "ci"
#   triggers: "generically named builder"
#   triggers: "linux ci builder"
#   gitiles: <
#     repo: "https://noop.com"
#     refs: "refs/heads/master"
#     refs: "refs/tags/blah"
#     refs: "regexp:refs/branch-heads/\\d+\\.\\d+"
#   >
# >
# acl_sets: <
#   name: "ci"
#   acls: <
#     role: OWNER
#     granted_to: "group:admins"
#   >
#   acls: <
#     granted_to: "group:all"
#   >
#   acls: <
#     role: TRIGGERER
#     granted_to: "group:devs"
#   >
# >
# ===
#
# === project.cfg
# name: "infra"
# access: "group:all"
# ===
