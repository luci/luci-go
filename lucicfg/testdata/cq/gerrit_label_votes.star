luci.project(name = "foo")

def cq_group(name, post_actions):
    luci.cq_group(
        name = name,
        watch = cq.refset("https://example.googlesource.com/repo1"),
        acls = [acl.entry(acl.CQ_COMMITTER, groups = ["committer"])],
        post_actions = post_actions,
    )

def test_duplicate_names():
    action = lambda name: cq.post_action_gerrit_label_votes(
        name = name,
        labels = {"foo": 1},
        conditions = [cq.post_action_triggering_condition(
            cq.MODE_DRY_RUN,
            [cq.STATUS_SUCCEEDED],
        )],
    )
    assert.fails(
        lambda: cq_group("main", [action("name-1"), action("name-1")]),
        "duplicate post_action name 'name-1'",
    )
    cq_group("main", [action("name-1"), action("name-2")])

test_duplicate_names()
