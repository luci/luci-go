luci.project(
    name = "proj",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
)

luci.recipe(
    name = "noop",
    cipd_package = "noop",
)

luci.bucket(name = "b1")
luci.bucket(name = "b2")

luci.builder(
    name = "b1 builder",
    bucket = "b1",
    executable = "noop",
    service_account = "noop@example.com",
)
luci.builder(
    name = "ambiguous builder",
    bucket = "b1",
    executable = "noop",
)
luci.builder(
    name = "b2 builder",
    bucket = "b2",
    executable = "noop",
)
luci.builder(
    name = "ambiguous builder",
    bucket = "b2",
    executable = "noop",
)

luci.gitiles_poller(
    name = "valid",
    bucket = "b1",
    repo = "https://noop.com",
    triggers = [
        "b1 builder",
        "b1/b1 builder",  # this is allowed
        "b2 builder",
        "b2/ambiguous builder",
    ],
)

luci.gitiles_poller(
    name = "ambiguous",
    bucket = "b1",
    repo = "https://noop.com",
    triggers = [
        "b1 builder",
        "ambiguous builder",  # error: is it b1 or b2?
    ],
)

luci.builder(
    name = "triggered",
    bucket = "b1",
    executable = "noop",
    triggered_by = [
        "b1 builder",
        "ambiguous builder",  # error: is it b1 or b2?
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/ambiguous_references.star: in <toplevel>
#   ...
# Error: ambiguous reference "ambiguous builder" in luci.gitiles_poller("b1/ambiguous"), possible variants:
#   luci.builder("b1/ambiguous builder")
#   luci.builder("b2/ambiguous builder")
#
# Traceback (most recent call last):
#   //errors/ambiguous_references.star: in <toplevel>
#   ...
# Error: ambiguous reference "ambiguous builder" in luci.builder("b1/triggered"), possible variants:
#   luci.builder("b1/ambiguous builder")
#   luci.builder("b2/ambiguous builder")
