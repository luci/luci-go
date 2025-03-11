luci.project(name = "foo")

def cq_group(name, tryjob_experiments):
    luci.cq_group(
        name = name,
        watch = cq.refset("https://example.googlesource.com/repo1"),
        acls = [acl.entry(acl.CQ_COMMITTER, groups = ["committer"])],
        tryjob_experiments = tryjob_experiments,
    )

def test_duplicate_experiment_names():
    exp = cq.tryjob_experiment(
        name = "infra.experiment.foo",
        owner_group_allowlist = ["googler"],
    )
    assert.fails(
        lambda: cq_group("main", [exp, exp]),
        "duplicate experiment name 'infra.experiment.foo'",
    )

test_duplicate_experiment_names()
