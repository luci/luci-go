luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    notify = "luci-notify.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)
luci.bucket(name = "bucket")
luci.recipe(name = "noop", cipd_package = "noop")

luci.notifier(
    name = "blamelist notifier",
    on_occurrence = ["FAILURE"],
    notify_blamelist = True,
    blamelist_repos_whitelist = ["https://repo.example.com"],
)

luci.notifier(
    name = "email notifier",
    on_occurrence = ["FAILURE"],
    notify_emails = ["a@example.com"],
)

luci.gitiles_poller(
    name = "p1",
    bucket = "bucket",
    repo = "https://polled.example.com",
)

luci.gitiles_poller(
    name = "p2",
    bucket = "bucket",
    repo = "https://polled.example.com",
)

luci.gitiles_poller(
    name = "p3",
    bucket = "bucket",
    repo = "https://another.example.com",
)

luci.builder(
    name = "explicit repo",
    bucket = "bucket",
    executable = "noop",
    repo = "https://repo.example.com",
    notifies = ["blamelist notifier", "email notifier"],
)

luci.builder(
    name = "derived through poller",
    bucket = "bucket",
    executable = "noop",
    triggered_by = ["p1", "p2"],
    notifies = ["blamelist notifier", "email notifier"],
)

luci.builder(
    name = "ambiguous pollers",
    bucket = "bucket",
    executable = "noop",
    triggered_by = ["p1", "p2", "p3"],
    notifies = ["blamelist notifier", "email notifier"],
)

luci.builder(
    name = "no repo at all",
    bucket = "bucket",
    executable = "noop",
    notifies = ["blamelist notifier", "email notifier"],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //notifiable/notify_blamelist_no_repo.star: in <toplevel>
#   ...
# Error: cannot deduce a primary repo for luci.builder("bucket/ambiguous pollers"), which is observed by a luci.notifier with notify_blamelist=True; add repo=... field
#
# Traceback (most recent call last):
#   //notifiable/notify_blamelist_no_repo.star: in <toplevel>
#   ...
# Error: cannot deduce a primary repo for luci.builder("bucket/no repo at all"), which is observed by a luci.notifier with notify_blamelist=True; add repo=... field
