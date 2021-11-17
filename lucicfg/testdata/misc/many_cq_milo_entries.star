luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    milo = "luci-milo.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)
luci.bucket(name = "bucket")
luci.recipe(name = "noop", cipd_package = "noop")

luci.cq_group(
    name = "cq group",
    watch = cq.refset("https://example.googlesource.com/repo"),
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["c"]),
    ],
)

luci.list_view(
    name = "list view",
)

def one(name):
    luci.builder(
        name = name,
        bucket = "bucket",
        executable = "noop",
        service_account = "noop@example.com",
    )
    luci.cq_tryjob_verifier(
        builder = name,
        cq_group = "cq group",
    )
    luci.list_view_entry(
        builder = name,
        list_view = "list view",
    )

# 11 builders, to make sure we cover the case when "10" is lexicographically
# before "2".
one("builder-k")
one("builder-j")
one("builder-i")
one("builder-h")
one("builder-g")
one("builder-f")
one("builder-e")
one("builder-d")
one("builder-c")
one("builder-b")
one("builder-a")

# commit-queue.cfg is in alphabetical order.
# luci-milo.cfg is in definition order.

# Expect configs:
#
# === commit-queue.cfg
# config_groups {
#   name: "cq group"
#   gerrit {
#     url: "https://example-review.googlesource.com"
#     projects {
#       name: "repo"
#       ref_regexp: "refs/heads/main"
#     }
#   }
#   verifiers {
#     gerrit_cq_ability {
#       committer_list: "c"
#     }
#     tryjob {
#       builders {
#         name: "project/bucket/builder-a"
#       }
#       builders {
#         name: "project/bucket/builder-b"
#       }
#       builders {
#         name: "project/bucket/builder-c"
#       }
#       builders {
#         name: "project/bucket/builder-d"
#       }
#       builders {
#         name: "project/bucket/builder-e"
#       }
#       builders {
#         name: "project/bucket/builder-f"
#       }
#       builders {
#         name: "project/bucket/builder-g"
#       }
#       builders {
#         name: "project/bucket/builder-h"
#       }
#       builders {
#         name: "project/bucket/builder-i"
#       }
#       builders {
#         name: "project/bucket/builder-j"
#       }
#       builders {
#         name: "project/bucket/builder-k"
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
# }
# ===
#
# === cr-buildbucket.cfg
# buckets {
#   name: "bucket"
#   swarming {
#     builders {
#       name: "builder-a"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop@example.com"
#     }
#     builders {
#       name: "builder-b"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop@example.com"
#     }
#     builders {
#       name: "builder-c"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop@example.com"
#     }
#     builders {
#       name: "builder-d"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop@example.com"
#     }
#     builders {
#       name: "builder-e"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop@example.com"
#     }
#     builders {
#       name: "builder-f"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop@example.com"
#     }
#     builders {
#       name: "builder-g"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop@example.com"
#     }
#     builders {
#       name: "builder-h"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop@example.com"
#     }
#     builders {
#       name: "builder-i"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop@example.com"
#     }
#     builders {
#       name: "builder-j"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop@example.com"
#     }
#     builders {
#       name: "builder-k"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop@example.com"
#     }
#   }
# }
# ===
#
# === luci-milo.cfg
# consoles {
#   id: "list view"
#   name: "list view"
#   builders {
#     name: "buildbucket/luci.project.bucket/builder-k"
#   }
#   builders {
#     name: "buildbucket/luci.project.bucket/builder-j"
#   }
#   builders {
#     name: "buildbucket/luci.project.bucket/builder-i"
#   }
#   builders {
#     name: "buildbucket/luci.project.bucket/builder-h"
#   }
#   builders {
#     name: "buildbucket/luci.project.bucket/builder-g"
#   }
#   builders {
#     name: "buildbucket/luci.project.bucket/builder-f"
#   }
#   builders {
#     name: "buildbucket/luci.project.bucket/builder-e"
#   }
#   builders {
#     name: "buildbucket/luci.project.bucket/builder-d"
#   }
#   builders {
#     name: "buildbucket/luci.project.bucket/builder-c"
#   }
#   builders {
#     name: "buildbucket/luci.project.bucket/builder-b"
#   }
#   builders {
#     name: "buildbucket/luci.project.bucket/builder-a"
#   }
#   builder_view_only: true
# }
# ===
#
# === project.cfg
# name: "project"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# realms {
#   name: "bucket"
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:noop@example.com"
#   }
# }
# ===
