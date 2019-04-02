luci.project(
    name = 'zzz',
    buildbucket = 'cr-buildbucket.appspot.com',
    scheduler = 'luci-scheduler.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)
luci.bucket(name = 'bucket')
luci.recipe(name = 'noop', cipd_package = 'noop')

luci.builder(
    name = 'triggered',
    bucket = 'bucket',
    executable = 'noop',
    triggered_by = [
        luci.builder(
            name = 'triggerer 1',
            bucket = 'bucket',
            executable = 'noop',
            service_account = 'noop@example.com',
        ),
        luci.builder(
            name = 'triggerer 2',
            bucket = 'bucket',
            executable = 'noop',
            service_account = 'noop@example.com',
        ),
    ],
)


luci.cq_group(
    name = 'group 1',
    watch = cq.refset('https://example.googlesource.com/repo1'),
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ['committer']),
    ],
    # This is fine, no ambiguities of what triggers "triggered".
    verifiers = [
        luci.cq_tryjob_verifier('triggered'),
        luci.cq_tryjob_verifier('triggerer 1'),
    ],
)


luci.cq_group(
    name = 'group 2',
    watch = cq.refset('https://example.googlesource.com/repo2'),
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ['committer']),
    ],
    # This is NOT fine, no clear winner for 'triggered_by' field in CQ config.
    verifiers = [
        luci.cq_tryjob_verifier('triggered'),
        luci.cq_tryjob_verifier('triggerer 1'),
        luci.cq_tryjob_verifier('triggerer 2'),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/cq/dup_triggered_by.star:53: in <toplevel>
#   ...
# Error: this builder is triggered by multiple builders in its CQ group which confuses CQ config generator
