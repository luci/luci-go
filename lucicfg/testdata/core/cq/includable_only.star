luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)
luci.bucket(name = "bucket")
luci.recipe(name = "noop", cipd_package = "noop")

luci.builder(
    name = "confused",
    bucket = "bucket",
    executable = "noop",
)

luci.cq_group(
    name = "main",
    watch = cq.refset("https://example.googlesource.com/repo1"),
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committer"]),
    ],
    verifiers = [
        luci.cq_tryjob_verifier(
            builder = "confused",
            includable_only = True,
            experiment_percentage = 10.0,
        ),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //cq/includable_only.star: in <toplevel>
#   ...
# Error: "includable_only" can not be used together with "experiment_percentage"
