load("@stdlib//internal/luci/lib/acl.star", "acl", "aclimpl")

def test_works():
    # Empty list is fine.
    assert.eq(aclimpl.normalize_acls([]), [])

    # Sorts and dedups.
    ents = [
        acl.entry(acl.BUILDBUCKET_OWNER, groups = ["group a", "group b"], users = "a@example.com"),
        acl.entry(acl.BUILDBUCKET_READER, users = "b@example.com"),
        acl.entry(acl.BUILDBUCKET_READER, users = "a@example.com"),
        acl.entry(acl.BUILDBUCKET_READER, users = ["a@example.com", "c@example.com"]),
        acl.entry(acl.BUILDBUCKET_READER),  # should be ignored
        acl.entry(acl.BUILDBUCKET_READER, groups = "group b"),
        acl.entry(acl.BUILDBUCKET_READER, groups = "group a"),
        acl.entry(acl.BUILDBUCKET_READER, projects = "p2"),
        acl.entry(acl.BUILDBUCKET_READER, projects = ["p1", "p2"]),
    ]
    assert.eq([(e.role, e.user, e.group, e.project) for e in aclimpl.normalize_acls(ents)], [
        (acl.BUILDBUCKET_OWNER, "a@example.com", None, None),
        (acl.BUILDBUCKET_OWNER, None, "group a", None),
        (acl.BUILDBUCKET_OWNER, None, "group b", None),
        (acl.BUILDBUCKET_READER, "a@example.com", None, None),
        (acl.BUILDBUCKET_READER, "b@example.com", None, None),
        (acl.BUILDBUCKET_READER, "c@example.com", None, None),
        (acl.BUILDBUCKET_READER, None, "group a", None),
        (acl.BUILDBUCKET_READER, None, "group b", None),
        (acl.BUILDBUCKET_READER, None, None, "p1"),
        (acl.BUILDBUCKET_READER, None, None, "p2"),
    ])

test_works()
