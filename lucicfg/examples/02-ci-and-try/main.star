#!/usr/bin/env lucicfg

"""A fully functional config example.

This example shows a fully functional config that defines several post-submit
(aka CI) and pre-submit (aka Try) builders. It also includes Milo console and
CQ build group definitions.

It is a good template for small projects that want to start using lucicfg.
"""

# Constants shared by multiple definitions below.
REPO_URL = "https://my-awesome-host.googlesource.com/my/awesome/repo"
RECIPE_BUNDLE = "infra/recipe_bundles/<your-recipe-bundle>"

# Definition of what LUCI micro-services to use and global ACLs that apply to
# all buckets.
luci.project(
    name = "my-awesome-project",
    buildbucket = "cr-buildbucket.appspot.com",
    logdog = "luci-logdog.appspot.com",
    milo = "luci-milo.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
    swarming = "chromium-swarm.appspot.com",
    acls = [
        # This project is publicly readable.
        acl.entry(
            roles = [
                acl.BUILDBUCKET_READER,
                acl.LOGDOG_READER,
                acl.PROJECT_CONFIGS_READER,
                acl.SCHEDULER_READER,
            ],
            groups = "all",
        ),
        # Allow committers to use CQ and to force-trigger and stop CI builds.
        acl.entry(
            roles = [
                acl.SCHEDULER_OWNER,
                acl.CQ_COMMITTER,
            ],
            groups = "my-awesome-project-committers",
        ),
        # Ability to launch CQ dry runs.
        acl.entry(
            roles = acl.CQ_DRY_RUNNER,
            groups = "my-awesome-project-tryjob-access",
        ),
        # Group with robots that have write access to the Logdog prefix.
        acl.entry(
            roles = acl.LOGDOG_WRITER,
            groups = "my-awesome-project-log-writers",
        ),
    ],
)

# Required Logdog configuration.
luci.logdog(gs_bucket = "my-awesome-project-logs-bucket")

# Optional tweaks.
luci.milo(
    logo = "https://storage.googleapis.com/my-awesome-project-resources/logo-200x200.png",
    favicon = "https://storage.googleapis.com/my-awesome-project-resources/favicon.icon",
)
luci.cq(status_host = "chromium-cq-status.appspot.com")

# Bucket with post-submit builders.
luci.bucket(name = "ci")

# Bucket with pre-submit builders.
luci.bucket(
    name = "try",
    acls = [
        # Allow launching tryjobs directly (in addition to doing it through CQ).
        acl.entry(
            roles = acl.BUILDBUCKET_TRIGGERER,
            groups = "my-awesome-project-tryjob-access",
        ),
    ],
)

# The Milo console with all post-submit builders, referenced below.
luci.console_view(
    name = "Main Console",
    repo = REPO_URL,
)

# The Milo builder list with all pre-submit builders, referenced below.
luci.list_view(
    name = "Try Builders",
)

# The CQ group with all pre-submit builders, referenced below.
luci.cq_group(
    name = "Main CQ",
    watch = cq.refset(REPO_URL),
    user_limits = [
        cq.user_limit(
            name = "user_limits_for_foo_and_bar",
            users = ["foo@example.com"],
            groups = ["bar"],
            run = cq.run_limits(max_active = 60, reach_limit_msg = "foobar"),
        ),
        cq.user_limit(
            name = "user_limits_for_committers",
            groups = ["committers"],
            run = cq.run_limits(max_active = 20),
        ),
        cq.user_limit(
            name = "user_limits_for_third_party_devs",
            users = [
                "partner_1@example.com",
                "partner_2@example.com",
            ],
            groups = ["third_party_devs_all"],
        ),
    ],
    user_limit_default = cq.user_limit(
        name = "default_user_quota",
    ),
    post_actions = [
        cq.post_action_gerrit_label_votes(
            name = "dry-run_verification_label",
            conditions = [
                cq.post_action_triggering_condition(
                    mode = cq.MODE_DRY_RUN,
                    statuses = [cq.STATUS_SUCCEEDED],
                ),
            ],
            labels = {
                "CQ-Verified": 1,
            },
        ),
        cq.post_action_gerrit_label_votes(
            name = "any-run-failures-and-cancellations",
            conditions = [
                cq.post_action_triggering_condition(
                    mode = m,
                    statuses = [cq.STATUS_FAILED, cq.STATUS_CANCELLED],
                )
                for m in [
                    cq.MODE_DRY_RUN,
                    cq.MODE_FULL_RUN,
                    cq.MODE_NEW_PATCHSET_RUN,
                ]
            ],
            labels = {
                "Code-Review": -1,
                "CQ-Verified": 0,
            },
        ),
    ],
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

# The gitiles poller: a source of commits that trigger CI builders.
luci.gitiles_poller(
    name = "my-awesome-project-poller",
    bucket = "ci",
    repo = REPO_URL,
)

def ci_builder(name, *, os, category, cpu = "x86-64"):
    """Defines a post-submit builder.

    Args:
      name: name of the builder to define.
      os: the target OS.
      category: the category to put it under in the console.
      cpu: the target CPU.
    """
    recipe_id = "ci_builder"

    luci.builder(
        name = name,
        bucket = "ci",
        executable = luci.recipe(
            name = recipe_id,
            recipe = "ci_builder",
            cipd_package = RECIPE_BUNDLE,
            use_python3 = True,
        ),
        dimensions = {
            "pool": "luci.my-awesome-project.ci",
            "os": os,
            "cpu": cpu,
        },
        service_account = "my-ci-builder@chops-service-accounts.iam.gserviceaccount.com",
        execution_timeout = 45 * time.minute,
        # Run this builder on commits to REPO_URL.
        triggered_by = ["my-awesome-project-poller"],
    )

    # Add it to the console as well.
    luci.console_view_entry(
        builder = "ci/" + name,  # disambiguate by prefixing the bucket name
        console_view = "Main Console",
        category = category,
    )

# Actually define a bunch of CI builders.
ci_builder("xenial", os = "Ubuntu-16.04", category = "Linux|16.04")
ci_builder("bionic", os = "Ubuntu-18.04", category = "Linux|18.04")
ci_builder("mac-10.13", os = "Mac-10.13", category = "Mac|10.13")
ci_builder(
    "win-32",
    os = "Windows",
    cpu = "x86-32",
    category = "Win|32",
)
ci_builder("win-64", os = "Windows", cpu = "x86-64", category = "Win|64")

def try_builder(name, *, os, cpu = "x86-64"):
    """Defines a pre-submit builder.

    Args:
      name: name of the builder to define.
      os: the target OS.
      cpu: the target CPU.
    """
    recipe_id = "try_builder"

    luci.builder(
        name = name,
        bucket = "try",
        executable = luci.recipe(
            name = recipe_id,
            recipe = "try_builder",
            cipd_package = RECIPE_BUNDLE,
            use_python3 = True,
        ),
        dimensions = {
            "pool": "luci.my-awesome-project.try",
            "os": os,
            "cpu": cpu,
        },
        service_account = "my-try-builder@chops-service-accounts.iam.gserviceaccount.com",
        execution_timeout = 45 * time.minute,
    )

    # Add to the CQ.
    luci.cq_tryjob_verifier(
        builder = "try/" + name,
        cq_group = "Main CQ",
    )

    # And also to the pre-submit builders list.
    luci.list_view_entry(
        builder = "try/" + name,
        list_view = "Try Builders",
    )

# Actually define a bunch of Try builders.
try_builder("xenial", os = "Ubuntu-16.04")
try_builder("bionic", os = "Ubuntu-18.04")
try_builder("mac-10.13", os = "Mac-10.13")
try_builder("win-32", os = "Windows", cpu = "x86-32")
try_builder("win-64", os = "Windows", cpu = "x86-64")
