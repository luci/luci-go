# The LUCI Scheduler Service

The LUCI Scheduler Service periodically makes URL fetches, runs Swarming tasks
or DM quests. It uses luci-config to fetch per-project lists of cron jobs. It
tries to prevent concurrent execution of job invocations (i.e. an invocation
will not start if previous one is still running).

It's built on top of App Engine Task Queues service.

TODO(vadimsh): Explain the design.
