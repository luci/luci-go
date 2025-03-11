luci.project(
    name = "proj",
    buildbucket = "cr-buildbucket-dev.appspot.com",
    logdog = "luci-logdog-dev.appspot.com",
    scheduler = "luci-scheduler-dev.appspot.com",
    swarming = "chromium-swarm-dev.appspot.com",
    acls = [
        acl.entry(
            roles = [
                acl.PROJECT_CONFIGS_READER,
                acl.BUILDBUCKET_READER,
                acl.SCHEDULER_READER,
            ],
            groups = ["readers1", "readers2"],
            users = ["r1@example.com", "r2@example.com"],
            projects = ["pr1", "pr2"],
        ),
        acl.entry(
            roles = [
                acl.BUILDBUCKET_TRIGGERER,
                acl.SCHEDULER_TRIGGERER,
            ],
            groups = "triggerers",
        ),
        acl.entry(
            roles = [
                acl.BUILDBUCKET_OWNER,
                acl.SCHEDULER_OWNER,
            ],
            groups = "owners",
        ),
        acl.entry(
            roles = [
                acl.LOGDOG_READER,
                acl.LOGDOG_WRITER,
            ],
            groups = "logdog",
        ),
        acl.entry(
            roles = acl.CQ_COMMITTER,
            groups = "committer",
        ),
        acl.entry(
            roles = acl.CQ_DRY_RUNNER,
            groups = "dry-runner",
        ),
    ],
)

luci.bucket(
    name = "bucket",
    acls = [
        acl.entry(
            roles = acl.BUILDBUCKET_OWNER,
            groups = "bucket-owner",
        ),
    ],
)

luci.builder(
    name = "builder",
    bucket = "bucket",
    executable = luci.recipe(
        name = "recipe",
        cipd_package = "recipe/bundles/main",
    ),
    service_account = "builder@example.com",
)

luci.builder(
    name = "cron",
    bucket = "bucket",
    executable = luci.recipe(
        name = "recipe",
        cipd_package = "recipe/bundles/main",
    ),
    service_account = "builder@example.com",
    schedule = "with 10s interval",
)

luci.gitiles_poller(
    name = "poller",
    bucket = "bucket",
    repo = "https://noop.com",
    refs = ["refs/heads/zzz"],
    schedule = "with 10s interval",
    triggers = ["builder"],
)

# Expect configs:
#
# === cr-buildbucket-dev.cfg
# buckets {
#   name: "bucket"
#   acls {
#     role: WRITER
#     group: "bucket-owner"
#   }
#   acls {
#     role: WRITER
#     group: "owners"
#   }
#   acls {
#     identity: "user:r1@example.com"
#   }
#   acls {
#     identity: "user:r2@example.com"
#   }
#   acls {
#     group: "readers1"
#   }
#   acls {
#     group: "readers2"
#   }
#   acls {
#     identity: "project:pr1"
#   }
#   acls {
#     identity: "project:pr2"
#   }
#   acls {
#     role: SCHEDULER
#     group: "triggerers"
#   }
#   swarming {
#     builders {
#       name: "builder"
#       swarming_host: "chromium-swarm-dev.appspot.com"
#       recipe {
#         name: "recipe"
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "builder@example.com"
#     }
#     builders {
#       name: "cron"
#       swarming_host: "chromium-swarm-dev.appspot.com"
#       recipe {
#         name: "recipe"
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "builder@example.com"
#     }
#   }
# }
# ===
#
# === luci-scheduler-dev.cfg
# job {
#   id: "builder"
#   realm: "bucket"
#   acl_sets: "bucket"
#   buildbucket {
#     server: "cr-buildbucket-dev.appspot.com"
#     bucket: "luci.proj.bucket"
#     builder: "builder"
#   }
# }
# job {
#   id: "cron"
#   realm: "bucket"
#   schedule: "with 10s interval"
#   acl_sets: "bucket"
#   buildbucket {
#     server: "cr-buildbucket-dev.appspot.com"
#     bucket: "luci.proj.bucket"
#     builder: "cron"
#   }
# }
# trigger {
#   id: "poller"
#   realm: "bucket"
#   schedule: "with 10s interval"
#   acl_sets: "bucket"
#   triggers: "builder"
#   gitiles {
#     repo: "https://noop.com"
#     refs: "regexp:refs/heads/zzz"
#   }
# }
# acl_sets {
#   name: "bucket"
#   acls {
#     role: OWNER
#     granted_to: "group:owners"
#   }
#   acls {
#     granted_to: "r1@example.com"
#   }
#   acls {
#     granted_to: "r2@example.com"
#   }
#   acls {
#     granted_to: "group:readers1"
#   }
#   acls {
#     granted_to: "group:readers2"
#   }
#   acls {
#     granted_to: "project:pr1"
#   }
#   acls {
#     granted_to: "project:pr2"
#   }
#   acls {
#     role: TRIGGERER
#     granted_to: "group:triggerers"
#   }
# }
# ===
#
# === project.cfg
# name: "proj"
# access: "user:r1@example.com"
# access: "user:r2@example.com"
# access: "group:readers1"
# access: "group:readers2"
# access: "project:pr1"
# access: "project:pr2"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
#   bindings {
#     role: "role/buildbucket.owner"
#     principals: "group:owners"
#   }
#   bindings {
#     role: "role/buildbucket.reader"
#     principals: "group:readers1"
#     principals: "group:readers2"
#     principals: "project:pr1"
#     principals: "project:pr2"
#     principals: "user:r1@example.com"
#     principals: "user:r2@example.com"
#   }
#   bindings {
#     role: "role/buildbucket.triggerer"
#     principals: "group:triggerers"
#   }
#   bindings {
#     role: "role/configs.reader"
#     principals: "group:readers1"
#     principals: "group:readers2"
#     principals: "project:pr1"
#     principals: "project:pr2"
#     principals: "user:r1@example.com"
#     principals: "user:r2@example.com"
#   }
#   bindings {
#     role: "role/cq.committer"
#     principals: "group:committer"
#   }
#   bindings {
#     role: "role/cq.dryRunner"
#     principals: "group:dry-runner"
#   }
#   bindings {
#     role: "role/logdog.reader"
#     principals: "group:logdog"
#   }
#   bindings {
#     role: "role/logdog.writer"
#     principals: "group:logdog"
#   }
#   bindings {
#     role: "role/scheduler.owner"
#     principals: "group:owners"
#   }
#   bindings {
#     role: "role/scheduler.reader"
#     principals: "group:readers1"
#     principals: "group:readers2"
#     principals: "project:pr1"
#     principals: "project:pr2"
#     principals: "user:r1@example.com"
#     principals: "user:r2@example.com"
#   }
#   bindings {
#     role: "role/scheduler.triggerer"
#     principals: "group:triggerers"
#   }
# }
# realms {
#   name: "bucket"
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:builder@example.com"
#   }
#   bindings {
#     role: "role/buildbucket.owner"
#     principals: "group:bucket-owner"
#   }
# }
# ===
