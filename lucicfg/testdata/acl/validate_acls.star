load("@stdlib//internal/luci/lib/acl.star", "acl", "aclimpl")

def test_works():
    # Works in general.
    acls = [
        acl.entry(acl.BUILDBUCKET_READER),
        acl.entry(acl.BUILDBUCKET_OWNER),
    ]
    assert.eq(aclimpl.validate_acls(acls), acls)

    # None or [] is OK.
    assert.eq(aclimpl.validate_acls(None), [])
    assert.eq(aclimpl.validate_acls([]), [])

    # Wrong type is NOT ok.
    assert.fails(
        lambda: aclimpl.validate_acls(111),
        'bad "acls": got int, want list',
    )
    assert.fails(
        lambda: aclimpl.validate_acls([111]),
        'bad "acls": got int, want acl.entry',
    )

    # Checks project_level_only.
    assert.true(acl.PROJECT_CONFIGS_READER.project_level_only)
    acls = acls + [acl.entry(acl.PROJECT_CONFIGS_READER)]
    assert.eq(aclimpl.validate_acls(acls, project_level = True), acls)
    assert.fails(
        lambda: aclimpl.validate_acls(acls),
        "role PROJECT_CONFIGS_READER can only be set at the project level",
    )

    # Checks allowed_roles.
    acls = [acl.entry(acl.BUILDBUCKET_READER)]
    assert.eq(aclimpl.validate_acls(acls, allowed_roles = [acl.BUILDBUCKET_READER]), acls)
    assert.fails(
        lambda: aclimpl.validate_acls(acls, allowed_roles = [acl.BUILDBUCKET_OWNER]),
        "role BUILDBUCKET_READER is not allowed in this context",
    )

test_works()
