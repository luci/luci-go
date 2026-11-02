luci.project(name = "foo")

luci.cq_group(
    name = "main",
    watch = [cq.refset("https://example.googlesource.com/proj/repo")],
    acls = [acl.entry(acl.CQ_COMMITTER, groups = ["committers"])],
    user_limit_default = cq.user_limit(
        name = "default_limit",
        run = cq.run_limits(max_active = 12),
        overrides = [
            # Weekday peak hours: tighter limit.
            cq.user_limit_override(
                schedule = cq.user_limit_schedule(
                    timezone = "America/Los_Angeles",
                    days = "Mon-Fri",
                    start_time = "08:00",
                    duration = 12 * time.hour,
                ),
                run = cq.run_limits(max_active = 6),
            ),
            # Overnight window that extends past midnight.
            cq.user_limit_override(
                schedule = cq.user_limit_schedule(
                    timezone = "UTC",
                    days = "Fri",
                    start_time = "20:30",
                    duration = 11 * time.hour + 30 * time.minute,
                ),
                run = cq.run_limits(max_active = 20),
            ),
            # Whole weekend: omitted `run` means unlimited.
            cq.user_limit_override(
                schedule = cq.user_limit_schedule(
                    timezone = "UTC",
                    days = "Sat,Sun",
                    start_time = "00:00",
                    duration = 24 * time.hour,
                ),
            ),
        ],
    ),
)

def schedule(**kwargs):
    args = dict(
        timezone = "UTC",
        days = "Mon-Fri",
        start_time = "08:00",
        duration = 12 * time.hour,
    )
    args.update(kwargs)
    return cq.user_limit_schedule(**args)

# Day specs are parsed with time.days_of_week(...).
assert.eq(schedule(days = "Mon-Fri").days, [1, 2, 3, 4, 5])
assert.eq(schedule(days = "sun, Sat").days, [6, 7])

assert.fails(
    lambda: schedule(timezone = "Mars/Olympus_Mons"),
    "unknown time zone Mars/Olympus_Mons",
)
assert.fails(lambda: schedule(timezone = "Local"), '"Local" is not allowed')
assert.fails(lambda: schedule(days = ""), 'bad "days": must not be empty')
assert.fails(lambda: schedule(days = ","), 'bad "days": must specify at least one day of the week')
assert.fails(lambda: schedule(days = ["Mon"]), 'bad "days": got list, want string')
assert.fails(lambda: schedule(days = "Funday"), "not a valid 3-char abbreviated day of the week")
assert.fails(lambda: schedule(start_time = None), 'missing required field "start_time"')
assert.fails(lambda: schedule(start_time = "24:00"), 'bad "start_time"')
assert.fails(lambda: schedule(start_time = "8:00"), 'bad "start_time"')
assert.fails(lambda: schedule(duration = None), 'missing required field "duration"')
assert.fails(lambda: schedule(duration = 0), 'bad "duration"')
assert.fails(lambda: schedule(duration = time.zero), 'bad "duration"')
assert.fails(lambda: schedule(duration = 24 * time.hour + time.second), 'bad "duration"')
assert.fails(lambda: schedule(duration = 1500 * time.millisecond), 'bad "duration"')
assert.fails(
    lambda: cq.user_limit_override(run = cq.run_limits(max_active = 1)),
    'missing required field "schedule"',
)
assert.fails(
    lambda: cq.user_limit_override(schedule = "not a schedule"),
    'bad "schedule"',
)
assert.fails(
    lambda: cq.user_limit(name = "x", overrides = [schedule()]),
    'bad "overrides\\[0\\]"',
)

# Expect configs:
#
# === commit-queue.cfg
# config_groups {
#   name: "main"
#   gerrit {
#     url: "https://example-review.googlesource.com"
#     projects {
#       name: "proj/repo"
#       ref_regexp: "refs/heads/main"
#     }
#   }
#   verifiers {
#     gerrit_cq_ability {
#       committer_list: "committers"
#     }
#   }
#   user_limit_default {
#     name: "default_limit"
#     run {
#       max_active {
#         value: 12
#       }
#     }
#     overrides {
#       schedule {
#         timezone: "America/Los_Angeles"
#         days: MONDAY
#         days: TUESDAY
#         days: WEDNESDAY
#         days: THURSDAY
#         days: FRIDAY
#         start_time {
#           hours: 8
#         }
#         duration {
#           seconds: 43200
#         }
#       }
#       run {
#         max_active {
#           value: 6
#         }
#       }
#     }
#     overrides {
#       schedule {
#         timezone: "UTC"
#         days: FRIDAY
#         start_time {
#           hours: 20
#           minutes: 30
#         }
#         duration {
#           seconds: 41400
#         }
#       }
#       run {
#         max_active {
#           value: 20
#         }
#       }
#     }
#     overrides {
#       schedule {
#         timezone: "UTC"
#         days: SATURDAY
#         days: SUNDAY
#         start_time {}
#         duration {
#           seconds: 86400
#         }
#       }
#       run {
#         max_active {
#           unlimited: true
#         }
#       }
#     }
#   }
# }
# ===
#
# === project.cfg
# name: "foo"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# ===
