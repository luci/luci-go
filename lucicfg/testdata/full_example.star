lucicfg.config(config_dir = ".output")
lucicfg.config(tracked_files = ["*.cfg"])
lucicfg.config(fail_on_warnings = True)

luci.project(
    name = "infra",
    buildbucket = "cr-buildbucket.appspot.com",
    logdog = "luci-logdog.appspot.com",
    milo = "luci-milo.appspot.com",
    notify = "luci-notify.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
    swarming = "chromium-swarm.appspot.com",
    tricium = "tricium-prod.appspot.com",
    acls = [
        acl.entry(
            roles = [
                acl.PROJECT_CONFIGS_READER,
                acl.LOGDOG_READER,
                acl.BUILDBUCKET_READER,
                acl.SCHEDULER_READER,
            ],
            groups = ["all"],
        ),
        acl.entry(
            roles = [
                acl.BUILDBUCKET_OWNER,
                acl.SCHEDULER_OWNER,
                acl.CQ_COMMITTER,
            ],
            groups = ["admins"],
        ),
    ],
)

luci.logdog(
    gs_bucket = "chromium-luci-logdog",
    cloud_logging_project = "chromium-build-logs",
)

luci.milo(
    logo = "https://storage.googleapis.com/chrome-infra-public/logo/chrome-infra-logo-200x200.png",
    favicon = "https://storage.googleapis.com/chrome-infra-public/logo/favicon.ico",
    bug_url_template = "https://bugs.chromium.org/p/tutu%%2C%%20all%%20aboard/issues/entry?summary=Bug%%20summary&description=Everything%%20is%%20broken&components=Stuff%%3EHard",
)

luci.buildbucket_notification_topic(
    name = "projects/my-cloud-project1/topics/my-topic1",
)
luci.buildbucket_notification_topic(
    name = "projects/my-cloud-project2/topics/my-topic2",
    compression = "ZSTD",
)

# Recipes.

luci.recipe(
    name = "main/recipe",
    cipd_package = "recipe/bundles/main",
    use_python3 = True,
    wrapper = ["path/to/wrapper"],
)

# Executables.

luci.executable(
    name = "main/executable",
    cipd_package = "executable/bundles/main",
    cmd = ["cmd"],
)

# Task Backend
luci.task_backend(
    name = "my_test_backend",
    target = "swarming://chromium",
    config = {"key": "value"},
)

# CI bucket.

luci.bucket(
    name = "ci",

    # Allow developers to force-launch CI builds through Scheduler, but not
    # directly through Buildbucket. The direct access to Buildbucket allows to
    # override almost all aspects of the builds (e.g. what recipe is used),
    # and Buildbucket totally ignores any concurrency limitations set in the
    # LUCI Scheduler configs. This makes direct Buildbucket access to CI buckets
    # dangerous. They usually have very small pool of machines, and these
    # machines are assumed to be running only "approved" code (being post-submit
    # builders).
    acls = [
        acl.entry(
            acl.SCHEDULER_TRIGGERER,
            groups = ["devs"],
            projects = ["some-project"],
        ),
    ],
)

luci.gitiles_poller(
    name = "main-poller",
    bucket = "ci",
    repo = "https://noop.com",
    refs = [
        "refs/heads/main",
        "refs/tags/blah",
        r"refs/branch-heads/\d+\.\d+",
    ],
    path_regexps = [".*"],
    path_regexps_exclude = ["excluded"],
    schedule = "with 10s interval",
)

luci.builder(
    name = "linux ci builder",
    bucket = "ci",
    description_html = "this is a linux ci builder",
    executable = "main/recipe",
    triggered_by = ["main-poller"],
    triggers = [
        "ci/generically named builder",
        "ci/generically named executable builder",
    ],
    properties = {
        "prop2": ["val2", 123],
        "prop1": "val1",
    },
    allowed_property_overrides = ["prop1"],
    service_account = "builder@example.com",
    caches = [
        swarming.cache("path1"),
        swarming.cache("path2", name = "name2"),
        swarming.cache("path3", name = "name3", wait_for_warm_cache = 10 * time.minute),
    ],
    execution_timeout = 3 * time.hour,
    grace_period = 2 * time.minute,
    heartbeat_timeout = 10 * time.minute,
    dimensions = {
        "os": "Linux",
        "builder": "linux ci builder",  # no auto_builder_dimension
        "prefer_if_available": [
            swarming.dimension("first-choice", expiration = 5 * time.minute),
            swarming.dimension("fallback"),
        ],
    },
    priority = 80,
    swarming_tags = ["tag1:val1", "tag2:val2"],
    expiration_timeout = time.hour,
    build_numbers = True,
    triggering_policy = scheduler.greedy_batching(
        max_concurrent_invocations = 5,
        max_batch_size = 10,
    ),
    resultdb_settings = resultdb.settings(
        enable = True,
        bq_exports = [
            resultdb.export_test_results(
                bq_table = ("luci-resultdb", "my-awesome-project", "all_test_results"),
                predicate = resultdb.test_result_predicate(
                    test_id_regexp = "ninja:.*",
                    unexpected_only = True,
                    variant_contains = True,
                    variant = {"test_suite": "super_interesting_suite"},
                ),
            ),
            resultdb.export_text_artifacts(
                bq_table = "luci-resultdb.my-awesome-project.all_text_artifacts",
                predicate = resultdb.artifact_predicate(
                    test_result_predicate = resultdb.test_result_predicate(
                        test_id_regexp = "ninja:.*",
                        unexpected_only = True,
                        variant_contains = True,
                        variant = {"test_suite": "super_interesting_suite"},
                    ),
                    included_invocations = False,
                    test_results = True,
                    content_type_regexp = "text.*",
                ),
            ),
        ],
        history_options = resultdb.history_options(
            by_timestamp = True,
        ),
    ),
    test_presentation = resultdb.test_presentation(
        column_keys = ["v.gpu"],
        grouping_keys = ["status", "v.test_suite"],
    ),
)

luci.builder(
    name = "generically named builder",
    bucket = "ci",
    executable = "main/recipe",
    triggered_by = ["main-poller"],
)

luci.builder(
    name = "generically named executable builder",
    bucket = "ci",
    executable = "main/executable",
    properties = {
        "prop2": ["val2", 123],
        "prop1": "val1",
    },
    allowed_property_overrides = ["*"],
    triggered_by = ["main-poller"],
)

luci.builder(
    name = "cron builder",
    bucket = "ci",
    executable = "main/recipe",
    schedule = "0 6 * * *",
    repo = "https://cron.repo.example.com",
)

luci.builder(
    name = "newest first schedule builder",
    bucket = "ci",
    executable = "main/recipe",
    triggering_policy = scheduler.newest_first(
        max_concurrent_invocations = 2,
        pending_timeout = 3*24*time.hour,
    ),
    repo = "https://newest-first.repo.example.com",
)

luci.builder(
    name = "builder with custom swarming host",
    bucket = "ci",
    executable = "main/recipe",
    swarming_host = "another-swarming.appspot.com",
)

luci.builder(
    name = "builder with the default test presentation config",
    bucket = "ci",
    executable = "main/recipe",
    test_presentation = resultdb.test_presentation(
        column_keys = [],
        grouping_keys = ["status"],
    ),
)

# Try bucket.

luci.bucket(
    name = "try",

    # Allow developers to launch try jobs directly with whatever parameters
    # they want. Try bucket is basically a free build farm for all developers.
    acls = [
        acl.entry(acl.BUILDBUCKET_TRIGGERER, groups = "devs"),
    ],
)

luci.builder(
    name = "linux try builder",
    bucket = "try",
    executable = "main/recipe",
)

luci.builder(
    name = "linux try builder 2",
    bucket = "try",
    executable = "main/recipe",
)

luci.builder(
    name = "generically named builder",
    bucket = "try",
    executable = "main/recipe",
)

luci.builder(
    name = "builder with executable",
    bucket = "try",
    executable = "main/executable",
)

luci.builder(
    name = "builder with experiment map",
    bucket = "try",
    executable = "main/executable",
    experiments = {
        "luci.enable_new_beta_feature": 32,
    },
)
luci.builder(
    name = "website-preview",
    bucket = "try",
    executable = "main/recipe",
)

luci.builder(
    name = "spell-checker",
    bucket = "try",
    executable = "main/recipe",
)

# Inline definitions.

def inline_poller():
    return luci.gitiles_poller(
        name = "inline poller",
        bucket = "inline",
        repo = "https://noop.com",
        refs = [
            "refs/heads/main",
            "refs/tags/blah",
            r"refs/branch-heads/\d+\.\d+",
        ],
        schedule = "with 10s interval",
    )

luci.builder(
    name = "triggerer builder",
    bucket = luci.bucket(name = "inline"),
    executable = luci.recipe(
        name = "inline/recipe",
        cipd_package = "recipe/bundles/inline",
        use_bbagent = True,
    ),
    service_account = "builder@example.com",
    triggers = [
        luci.builder(
            name = "triggered builder",
            bucket = "inline",
            executable = "inline/recipe",
        ),
    ],
    triggered_by = [inline_poller()],
)

luci.builder(
    name = "another builder",
    bucket = "inline",
    executable = luci.recipe(
        name = "inline/recipe",
        cipd_package = "recipe/bundles/inline",
        use_bbagent = True,
    ),
    service_account = "builder@example.com",
    triggered_by = [inline_poller()],
)

luci.builder(
    name = "another executable builder",
    bucket = "inline",
    executable = luci.executable(
        name = "inline/executable",
        cipd_package = "executable/bundles/inline",
    ),
    service_account = "builder@example.com",
    triggered_by = [inline_poller()],
)

luci.builder(
    name = "executable builder wrapper",
    bucket = "inline",
    executable = luci.executable(
        name = "wrapped executable",
        cipd_package = "executable/bundles/inline",
        wrapper = ["/path/to/wrapper"],
    ),
    service_account = "builder@example.com",
)

# List views.

luci.list_view(
    name = "List view",
    entries = [
        "cron builder",
        "ci/generically named builder",
        luci.list_view_entry(
            builder = "linux ci builder",
        ),
    ],
)

luci.list_view_entry(
    list_view = "List view",
    builder = "inline/triggered builder",
)

# Console views.

luci.console_view(
    name = "Console view",
    title = "CI Builders",
    header = {
        "links": [
            {"name": "a", "links": [{"text": "a"}]},
            {"name": "b", "links": [{"text": "b"}]},
        ],
    },
    repo = "https://noop.com",
    refs = ["refs/tags/blah", r"refs/branch-heads/\d+\.\d+"],
    exclude_ref = "refs/heads/main",
    include_experimental_builds = True,
    entries = [
        luci.console_view_entry(
            builder = "linux ci builder",
            category = "a|b",
            short_name = "lnx",
        ),
        # An alias for luci.console_view_entry(**{...}).
        {"builder": "cron builder", "category": "cron"},
    ],
    default_commit_limit = 3,
    default_expand = True,
)

luci.console_view_entry(
    console_view = "Console view",
    builder = "inline/triggered builder",
)

luci.external_console_view(
    name = "external-console",
    title = "External console",
    source = "chromium:main",
)

# Notifier.

luci.notifier(
    name = "main notifier",
    on_new_status = ["FAILURE"],
    notify_emails = ["someone@example,com"],
    notify_blamelist = True,
    template = "notifier_template",
    notified_by = [
        "linux ci builder",
        "cron builder",
    ],
)

luci.notifier_template(
    name = "notifier_template",
    body = "Hello\n\nHi\n",
)

luci.notifier_template(
    name = "another_template",
    body = "Boo!\n",
)

luci.builder(
    name = "watched builder",
    bucket = "ci",
    executable = "main/recipe",
    repo = "https://custom.example.com/repo",
    notifies = ["main notifier"],
)

# CQ.

luci.cq(
    submit_max_burst = 10,
    submit_burst_delay = 10 * time.minute,
    draining_start_time = "2017-12-23T15:47:58Z",
    status_host = "chromium-cq-status.appspot.com",
)

luci.cq_group(
    name = "main-cq",
    watch = [
        cq.refset("https://example.googlesource.com/repo"),
        cq.refset(
            "https://example.googlesource.com/another/repo",
            refs_exclude = ["refs/heads/infra/config"],
        ),
    ],
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committers"]),
        acl.entry(acl.CQ_DRY_RUNNER, groups = ["dry-runners"]),
        acl.entry(acl.CQ_NEW_PATCHSET_RUN_TRIGGERER, groups = ["new-patchset-runners"]),
    ],
    allow_submit_with_open_deps = True,
    allow_owner_if_submittable = cq.ACTION_COMMIT,
    trust_dry_runner_deps = True,
    tree_status_host = "tree-status.example.com",
    verifiers = [
        luci.cq_tryjob_verifier(
            builder = "linux try builder",
            cancel_stale = False,
            result_visibility = cq.COMMENT_LEVEL_RESTRICTED,
            location_filters = [
                cq.location_filter(
                    gerrit_host_regexp = "example.com",
                    gerrit_project_regexp = "repo",
                    path_regexp = "all/one.txt",
                    exclude = True,
                ),
            ],
            mode_allowlist = [cq.MODE_DRY_RUN, cq.MODE_FULL_RUN],
        ),
        # An experimental verifier with a location filter.
        luci.cq_tryjob_verifier(
            builder = "linux try builder 2",
            location_filters = [
                cq.location_filter(
                    gerrit_host_regexp = "example.com",
                    gerrit_project_regexp = "repo",
                    path_regexp = "all/two.txt",
                    exclude = True,
                ),
            ],
            experiment_percentage = 50,
        ),
        # An alias for luci.cq_tryjob_verifier(**{...}).
        {"builder": "try/generically named builder", "disable_reuse": True},
        # An alias for luci.cq_tryjob_verifier(<builder>).
        "another-project:try/yyy",
        luci.cq_tryjob_verifier(
            builder = "another-project:try/zzz",
            includable_only = True,
            owner_whitelist = ["another-project-committers"],
        ),
        luci.cq_tryjob_verifier(
            builder = "website-preview",
            owner_whitelist = ["project-contributor"],
            mode_allowlist = [cq.MODE_NEW_PATCHSET_RUN],
        ),
        luci.cq_tryjob_verifier(
            builder = "spell-checker",
            owner_whitelist = ["project-contributor"],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
            disable_reuse = True,
        ),
    ],
    additional_modes = cq.run_mode(
        name = "TEST_RUN",
        cq_label_value = 1,
        triggering_label = "TEST_RUN_LABEL",
        triggering_value = 1,
    ),
    user_limits = [
        cq.user_limit(
            name = "comitters",
            groups = ["comitters"],
            run = cq.run_limits(max_active = 3),
        ),
    ],
    user_limit_default = cq.user_limit(
        name = "user_limit_default",
        run = cq.run_limits(max_active = 1),
    ),
    tryjob_experiments = [
        cq.tryjob_experiment(
            name = "infra.experiment.internal",
            owner_group_allowlist = ["googler", "bot-accounts"],
        ),
        cq.tryjob_experiment(
            name = "infra.experiment.public",
        ),
    ],
)

luci.cq_tryjob_verifier(
    builder = "triggerer builder",
    cq_group = "main-cq",
    experiment_percentage = 50.0,
)

luci.cq_tryjob_verifier(
    builder = luci.builder(
        name = "main cq builder",
        bucket = "try",
        executable = "main/recipe",
    ),
    equivalent_builder = luci.builder(
        name = "equivalent cq builder",
        bucket = "try",
        executable = "main/recipe",
    ),
    equivalent_builder_percentage = 60,
    equivalent_builder_whitelist = "owners",
    cq_group = "main-cq",
)

luci.cq_tryjob_verifier(
    builder = "another-project:analyzer/format checker",
    cq_group = "main-cq",
    location_filters = [
        cq.location_filter(path_regexp = ".+\\.py"),
        cq.location_filter(path_regexp = ".+\\.go"),
        cq.location_filter(path_regexp = ".+\\.X4"),
    ],
    owner_whitelist = ["project-contributor"],
    mode_allowlist = [cq.MODE_ANALYZER_RUN, cq.MODE_FULL_RUN],
)

# Emitting arbitrary configs,

lucicfg.emit(
    dest = "dir/custom.cfg",
    data = "hello!\n",
)

# Expect configs:
#
# === commit-queue.cfg
# draining_start_time: "2017-12-23T15:47:58Z"
# cq_status_host: "chromium-cq-status.appspot.com"
# submit_options {
#   max_burst: 10
#   burst_delay {
#     seconds: 600
#   }
# }
# config_groups {
#   name: "main-cq"
#   gerrit {
#     url: "https://example-review.googlesource.com"
#     projects {
#       name: "repo"
#       ref_regexp: "refs/heads/main"
#     }
#     projects {
#       name: "another/repo"
#       ref_regexp: "refs/heads/main"
#       ref_regexp_exclude: "refs/heads/infra/config"
#     }
#   }
#   verifiers {
#     gerrit_cq_ability {
#       committer_list: "admins"
#       committer_list: "committers"
#       dry_run_access_list: "dry-runners"
#       new_patchset_run_access_list: "new-patchset-runners"
#       allow_submit_with_open_deps: true
#       allow_owner_if_submittable: COMMIT
#       trust_dry_runner_deps: true
#     }
#     tree_status {
#       url: "https://tree-status.example.com"
#     }
#     tryjob {
#       builders {
#         name: "another-project/analyzer/format checker"
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*"
#           path_regexp: ".+\\.py"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*"
#           path_regexp: ".+\\.go"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*"
#           path_regexp: ".+\\.X4"
#         }
#         owner_whitelist_group: "project-contributor"
#         mode_allowlist: "ANALYZER_RUN"
#         mode_allowlist: "FULL_RUN"
#       }
#       builders {
#         name: "another-project/try/yyy"
#       }
#       builders {
#         name: "another-project/try/zzz"
#         includable_only: true
#         owner_whitelist_group: "another-project-committers"
#       }
#       builders {
#         name: "infra/inline/triggerer builder"
#         experiment_percentage: 50
#       }
#       builders {
#         name: "infra/try/generically named builder"
#         disable_reuse: true
#       }
#       builders {
#         name: "infra/try/linux try builder"
#         result_visibility: COMMENT_LEVEL_RESTRICTED
#         cancel_stale: NO
#         location_filters {
#           gerrit_host_regexp: "example.com"
#           gerrit_project_regexp: "repo"
#           path_regexp: "all/one.txt"
#           exclude: true
#         }
#         mode_allowlist: "DRY_RUN"
#         mode_allowlist: "FULL_RUN"
#       }
#       builders {
#         name: "infra/try/linux try builder 2"
#         experiment_percentage: 50
#         location_filters {
#           gerrit_host_regexp: "example.com"
#           gerrit_project_regexp: "repo"
#           path_regexp: "all/two.txt"
#           exclude: true
#         }
#       }
#       builders {
#         name: "infra/try/main cq builder"
#         equivalent_to {
#           name: "infra/try/equivalent cq builder"
#           percentage: 60
#           owner_whitelist_group: "owners"
#         }
#       }
#       builders {
#         name: "infra/try/spell-checker"
#         disable_reuse: true
#         owner_whitelist_group: "project-contributor"
#         mode_allowlist: "ANALYZER_RUN"
#       }
#       builders {
#         name: "infra/try/website-preview"
#         owner_whitelist_group: "project-contributor"
#         mode_allowlist: "NEW_PATCHSET_RUN"
#       }
#       retry_config {
#         single_quota: 1
#         global_quota: 2
#         failure_weight: 100
#         transient_failure_weight: 1
#         timeout_weight: 100
#       }
#     }
#   }
#   additional_modes {
#     name: "TEST_RUN"
#     cq_label_value: 1
#     triggering_label: "TEST_RUN_LABEL"
#     triggering_value: 1
#   }
#   user_limits {
#     name: "comitters"
#     principals: "group:comitters"
#     run {
#       max_active {
#         value: 3
#       }
#     }
#   }
#   user_limit_default {
#     name: "user_limit_default"
#     run {
#       max_active {
#         value: 1
#       }
#     }
#   }
#   tryjob_experiments {
#     name: "infra.experiment.internal"
#     condition {
#       owner_group_allowlist: "googler"
#       owner_group_allowlist: "bot-accounts"
#     }
#   }
#   tryjob_experiments {
#     name: "infra.experiment.public"
#   }
# }
# ===
#
# === cr-buildbucket.cfg
# buckets {
#   name: "ci"
#   acls {
#     role: WRITER
#     group: "admins"
#   }
#   acls {
#     group: "all"
#   }
#   swarming {
#     builders {
#       name: "builder with custom swarming host"
#       swarming_host: "another-swarming.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "builder with the default test presentation config"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "cron builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "generically named builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "generically named executable builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "executable/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "cmd"
#       }
#       properties:
#         '{'
#         '  "prop1": "val1",'
#         '  "prop2": ['
#         '    "val2",'
#         '    123'
#         '  ]'
#         '}'
#       allowed_property_overrides: "*"
#     }
#     builders {
#       name: "linux ci builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       swarming_tags: "tag1:val1"
#       swarming_tags: "tag2:val2"
#       dimensions: "builder:linux ci builder"
#       dimensions: "os:Linux"
#       dimensions: "300:prefer_if_available:first-choice"
#       dimensions: "prefer_if_available:fallback"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "$recipe_engine/resultdb/test_presentation": {'
#         '    "column_keys": ['
#         '      "v.gpu"'
#         '    ],'
#         '    "grouping_keys": ['
#         '      "status",'
#         '      "v.test_suite"'
#         '    ]'
#         '  },'
#         '  "prop1": "val1",'
#         '  "prop2": ['
#         '    "val2",'
#         '    123'
#         '  ],'
#         '  "recipe": "main/recipe"'
#         '}'
#       allowed_property_overrides: "prop1"
#       priority: 80
#       execution_timeout_secs: 10800
#       heartbeat_timeout_secs: 600
#       expiration_secs: 3600
#       grace_period {
#         seconds: 120
#       }
#       caches {
#         name: "name2"
#         path: "path2"
#       }
#       caches {
#         name: "name3"
#         path: "path3"
#         wait_for_warm_cache_secs: 600
#       }
#       caches {
#         name: "path1"
#         path: "path1"
#       }
#       build_numbers: YES
#       service_account: "builder@example.com"
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#       resultdb {
#         enable: true
#         bq_exports {
#           project: "luci-resultdb"
#           dataset: "my-awesome-project"
#           table: "all_test_results"
#           test_results {
#             predicate {
#               test_id_regexp: "ninja:.*"
#               variant {
#                 contains {
#                   def {
#                     key: "test_suite"
#                     value: "super_interesting_suite"
#                   }
#                 }
#               }
#               expectancy: VARIANTS_WITH_UNEXPECTED_RESULTS
#             }
#           }
#         }
#         bq_exports {
#           project: "luci-resultdb"
#           dataset: "my-awesome-project"
#           table: "all_text_artifacts"
#           text_artifacts {
#             predicate {
#               follow_edges {
#                 test_results: true
#               }
#               test_result_predicate {
#                 test_id_regexp: "ninja:.*"
#                 variant {
#                   contains {
#                     def {
#                       key: "test_suite"
#                       value: "super_interesting_suite"
#                     }
#                   }
#                 }
#                 expectancy: VARIANTS_WITH_UNEXPECTED_RESULTS
#               }
#               content_type_regexp: "text.*"
#             }
#           }
#         }
#         history_options {
#           use_invocation_timestamp: true
#         }
#       }
#       description_html: "this is a linux ci builder"
#     }
#     builders {
#       name: "newest first schedule builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "watched builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#   }
# }
# buckets {
#   name: "inline"
#   acls {
#     role: WRITER
#     group: "admins"
#   }
#   acls {
#     group: "all"
#   }
#   swarming {
#     builders {
#       name: "another builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/inline"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#       }
#       properties:
#         '{'
#         '  "recipe": "inline/recipe"'
#         '}'
#       service_account: "builder@example.com"
#     }
#     builders {
#       name: "another executable builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "executable/bundles/inline"
#         cipd_version: "refs/heads/main"
#       }
#       properties: '{}'
#       service_account: "builder@example.com"
#     }
#     builders {
#       name: "executable builder wrapper"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "executable/bundles/inline"
#         cipd_version: "refs/heads/main"
#         wrapper: "/path/to/wrapper"
#       }
#       properties: '{}'
#       service_account: "builder@example.com"
#     }
#     builders {
#       name: "triggered builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/inline"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#       }
#       properties:
#         '{'
#         '  "recipe": "inline/recipe"'
#         '}'
#     }
#     builders {
#       name: "triggerer builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/inline"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#       }
#       properties:
#         '{'
#         '  "recipe": "inline/recipe"'
#         '}'
#       service_account: "builder@example.com"
#     }
#   }
# }
# buckets {
#   name: "try"
#   acls {
#     role: WRITER
#     group: "admins"
#   }
#   acls {
#     group: "all"
#   }
#   acls {
#     role: SCHEDULER
#     group: "devs"
#   }
#   swarming {
#     builders {
#       name: "builder with executable"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "executable/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "cmd"
#       }
#       properties: '{}'
#     }
#     builders {
#       name: "builder with experiment map"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "executable/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "cmd"
#       }
#       properties: '{}'
#       experiments {
#         key: "luci.enable_new_beta_feature"
#         value: 32
#       }
#     }
#     builders {
#       name: "equivalent cq builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "generically named builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "linux try builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "linux try builder 2"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "main cq builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "spell-checker"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "website-preview"
#       swarming_host: "chromium-swarm.appspot.com"
#       exe {
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#         cmd: "luciexe"
#         wrapper: "path/to/wrapper"
#       }
#       properties:
#         '{'
#         '  "recipe": "main/recipe"'
#         '}'
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#   }
# }
# common_config {
#   builds_notification_topics {
#     name: "projects/my-cloud-project1/topics/my-topic1"
#   }
#   builds_notification_topics {
#     name: "projects/my-cloud-project2/topics/my-topic2"
#     compression: ZSTD
#   }
# }
# ===
#
# === dir/custom.cfg
# hello!
# ===
#
# === luci-logdog.cfg
# reader_auth_groups: "all"
# archive_gs_bucket: "chromium-luci-logdog"
# cloud_logging_config {
#   destination: "chromium-build-logs"
# }
# ===
#
# === luci-milo.cfg
# consoles {
#   id: "List view"
#   name: "List view"
#   builders {
#     name: "buildbucket/luci.infra.ci/cron builder"
#   }
#   builders {
#     name: "buildbucket/luci.infra.ci/generically named builder"
#   }
#   builders {
#     name: "buildbucket/luci.infra.ci/linux ci builder"
#   }
#   builders {
#     name: "buildbucket/luci.infra.inline/triggered builder"
#   }
#   favicon_url: "https://storage.googleapis.com/chrome-infra-public/logo/favicon.ico"
#   builder_view_only: true
# }
# consoles {
#   id: "Console view"
#   name: "CI Builders"
#   repo_url: "https://noop.com"
#   refs: "regexp:refs/tags/blah"
#   refs: "regexp:refs/branch-heads/\\d+\\.\\d+"
#   exclude_ref: "refs/heads/main"
#   manifest_name: "REVISION"
#   builders {
#     name: "buildbucket/luci.infra.ci/linux ci builder"
#     category: "a|b"
#     short_name: "lnx"
#   }
#   builders {
#     name: "buildbucket/luci.infra.ci/cron builder"
#     category: "cron"
#   }
#   builders {
#     name: "buildbucket/luci.infra.inline/triggered builder"
#   }
#   favicon_url: "https://storage.googleapis.com/chrome-infra-public/logo/favicon.ico"
#   header {
#     links {
#       name: "a"
#       links {
#         text: "a"
#       }
#     }
#     links {
#       name: "b"
#       links {
#         text: "b"
#       }
#     }
#   }
#   include_experimental_builds: true
#   default_commit_limit: 3
#   default_expand: true
# }
# consoles {
#   id: "external-console"
#   name: "External console"
#   external_project: "chromium"
#   external_id: "main"
# }
# logo_url: "https://storage.googleapis.com/chrome-infra-public/logo/chrome-infra-logo-200x200.png"
# bug_url_template: "https://bugs.chromium.org/p/tutu%2C%20all%20aboard/issues/entry?summary=Bug%20summary&description=Everything%20is%20broken&components=Stuff%3EHard"
# ===
#
# === luci-notify.cfg
# notifiers {
#   notifications {
#     on_new_status: FAILURE
#     email {
#       recipients: "someone@example,com"
#     }
#     template: "notifier_template"
#     notify_blamelist {}
#   }
#   builders {
#     bucket: "ci"
#     name: "cron builder"
#     repository: "https://cron.repo.example.com"
#   }
# }
# notifiers {
#   notifications {
#     on_new_status: FAILURE
#     email {
#       recipients: "someone@example,com"
#     }
#     template: "notifier_template"
#     notify_blamelist {}
#   }
#   builders {
#     bucket: "ci"
#     name: "linux ci builder"
#     repository: "https://noop.com"
#   }
# }
# notifiers {
#   notifications {
#     on_new_status: FAILURE
#     email {
#       recipients: "someone@example,com"
#     }
#     template: "notifier_template"
#     notify_blamelist {}
#   }
#   builders {
#     bucket: "ci"
#     name: "watched builder"
#     repository: "https://custom.example.com/repo"
#   }
# }
# ===
#
# === luci-notify/email-templates/another_template.template
# Boo!
# ===
#
# === luci-notify/email-templates/notifier_template.template
# Hello
#
# Hi
# ===
#
# === luci-scheduler.cfg
# job {
#   id: "another builder"
#   realm: "inline"
#   acl_sets: "inline"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.infra.inline"
#     builder: "another builder"
#   }
# }
# job {
#   id: "another executable builder"
#   realm: "inline"
#   acl_sets: "inline"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.infra.inline"
#     builder: "another executable builder"
#   }
# }
# job {
#   id: "cron builder"
#   realm: "ci"
#   schedule: "0 6 * * *"
#   acl_sets: "ci"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.infra.ci"
#     builder: "cron builder"
#   }
# }
# job {
#   id: "generically named builder"
#   realm: "ci"
#   acls {
#     role: TRIGGERER
#     granted_to: "builder@example.com"
#   }
#   acl_sets: "ci"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.infra.ci"
#     builder: "generically named builder"
#   }
# }
# job {
#   id: "generically named executable builder"
#   realm: "ci"
#   acls {
#     role: TRIGGERER
#     granted_to: "builder@example.com"
#   }
#   acl_sets: "ci"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.infra.ci"
#     builder: "generically named executable builder"
#   }
# }
# job {
#   id: "linux ci builder"
#   realm: "ci"
#   acl_sets: "ci"
#   triggering_policy {
#     kind: GREEDY_BATCHING
#     max_concurrent_invocations: 5
#     max_batch_size: 10
#   }
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.infra.ci"
#     builder: "linux ci builder"
#   }
# }
# job {
#   id: "newest first schedule builder"
#   realm: "ci"
#   acl_sets: "ci"
#   triggering_policy {
#     kind: NEWEST_FIRST
#     max_concurrent_invocations: 2
#     pending_timeout {
#       seconds: 259200
#     }
#   }
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.infra.ci"
#     builder: "newest first schedule builder"
#   }
# }
# job {
#   id: "triggered builder"
#   realm: "inline"
#   acls {
#     role: TRIGGERER
#     granted_to: "builder@example.com"
#   }
#   acl_sets: "inline"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.infra.inline"
#     builder: "triggered builder"
#   }
# }
# job {
#   id: "triggerer builder"
#   realm: "inline"
#   acl_sets: "inline"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.infra.inline"
#     builder: "triggerer builder"
#   }
# }
# trigger {
#   id: "inline poller"
#   realm: "inline"
#   schedule: "with 10s interval"
#   acl_sets: "inline"
#   triggers: "another builder"
#   triggers: "another executable builder"
#   triggers: "triggerer builder"
#   gitiles {
#     repo: "https://noop.com"
#     refs: "regexp:refs/heads/main"
#     refs: "regexp:refs/tags/blah"
#     refs: "regexp:refs/branch-heads/\\d+\\.\\d+"
#   }
# }
# trigger {
#   id: "main-poller"
#   realm: "ci"
#   schedule: "with 10s interval"
#   acl_sets: "ci"
#   triggers: "generically named builder"
#   triggers: "generically named executable builder"
#   triggers: "linux ci builder"
#   gitiles {
#     repo: "https://noop.com"
#     refs: "regexp:refs/heads/main"
#     refs: "regexp:refs/tags/blah"
#     refs: "regexp:refs/branch-heads/\\d+\\.\\d+"
#     path_regexps: ".*"
#     path_regexps_exclude: "excluded"
#   }
# }
# acl_sets {
#   name: "ci"
#   acls {
#     role: OWNER
#     granted_to: "group:admins"
#   }
#   acls {
#     granted_to: "group:all"
#   }
#   acls {
#     role: TRIGGERER
#     granted_to: "group:devs"
#   }
#   acls {
#     role: TRIGGERER
#     granted_to: "project:some-project"
#   }
# }
# acl_sets {
#   name: "inline"
#   acls {
#     role: OWNER
#     granted_to: "group:admins"
#   }
#   acls {
#     granted_to: "group:all"
#   }
# }
# ===
#
# === project.cfg
# name: "infra"
# access: "group:all"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
#   bindings {
#     role: "role/buildbucket.owner"
#     principals: "group:admins"
#   }
#   bindings {
#     role: "role/buildbucket.reader"
#     principals: "group:all"
#   }
#   bindings {
#     role: "role/configs.reader"
#     principals: "group:all"
#   }
#   bindings {
#     role: "role/cq.committer"
#     principals: "group:admins"
#   }
#   bindings {
#     role: "role/logdog.reader"
#     principals: "group:all"
#   }
#   bindings {
#     role: "role/scheduler.owner"
#     principals: "group:admins"
#   }
#   bindings {
#     role: "role/scheduler.reader"
#     principals: "group:all"
#   }
# }
# realms {
#   name: "ci"
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:builder@example.com"
#   }
#   bindings {
#     role: "role/scheduler.triggerer"
#     principals: "group:devs"
#     principals: "project:some-project"
#   }
# }
# realms {
#   name: "inline"
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:builder@example.com"
#   }
# }
# realms {
#   name: "try"
#   bindings {
#     role: "role/buildbucket.triggerer"
#     principals: "group:devs"
#   }
# }
# ===
#
# === tricium-prod.cfg
# functions {
#   type: ANALYZER
#   name: "AnotherProjectAnalyzerFormatChecker"
#   needs: GIT_FILE_DETAILS
#   provides: RESULTS
#   path_filters: "*.X4"
#   path_filters: "*.go"
#   path_filters: "*.py"
#   impls {
#     provides_for_platform: LINUX
#     runtime_platform: LINUX
#     recipe {
#       project: "another-project"
#       bucket: "analyzer"
#       builder: "format checker"
#     }
#   }
# }
# functions {
#   type: ANALYZER
#   name: "InfraTrySpellChecker"
#   needs: GIT_FILE_DETAILS
#   provides: RESULTS
#   impls {
#     provides_for_platform: LINUX
#     runtime_platform: LINUX
#     recipe {
#       project: "infra"
#       bucket: "try"
#       builder: "spell-checker"
#     }
#   }
# }
# selections {
#   function: "AnotherProjectAnalyzerFormatChecker"
#   platform: LINUX
# }
# selections {
#   function: "InfraTrySpellChecker"
#   platform: LINUX
# }
# repos {
#   gerrit_project {
#     host: "example-review.googlesource.com"
#     project: "another/repo"
#     git_url: "https://example.googlesource.com/another/repo"
#   }
#   whitelisted_group: "project-contributor"
#   check_all_revision_kinds: true
# }
# repos {
#   gerrit_project {
#     host: "example-review.googlesource.com"
#     project: "repo"
#     git_url: "https://example.googlesource.com/repo"
#   }
#   whitelisted_group: "project-contributor"
#   check_all_revision_kinds: true
# }
# service_account: "tricium-prod@appspot.gserviceaccount.com"
# ===
