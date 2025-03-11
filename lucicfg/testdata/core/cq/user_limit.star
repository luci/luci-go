luci.project(name = "foo")

# name character checks
def fail(name, msg):
    assert.fails(lambda: cq.user_limit(name = name), msg)

cq.user_limit(name = "this_is_a_good_name", users = ["foo@example.com"])
cq.user_limit(name = "foo-abc@example.com", groups = ["engineers"])
fail("", "must not be empty")
fail("@a", "should match \"\\^\\[")
fail("Abc Def", "should match \"\\^\\[")

# name uniqueness checks
def cq_group(name, user_limits, user_limits_default):
    luci.cq_group(
        name = name,
        watch = [cq.refset("https://example.googlesource.com/proj/repo")],
        acls = [acl.entry(acl.CQ_COMMITTER, groups = ["committers"])],
        user_limits = user_limits,
        user_limit_default = user_limits_default,
    )

p1 = cq.user_limit(name = "user_limits", users = ["someone@example.com"])
p2 = cq.user_limit(name = "group_limits", groups = ["abc"])
p3 = cq.user_limit(name = "no_principal_limits")

# cq.user_limit(s) can be passed to multiple cq_group(s),
#
# However, it doesn't mean that runs and tryjobs of all the cq_group(s) will be
# tracked together under the same limit. runs and tryjobs are tracked separately
# for each cq_group.
cq_group("cq 1", [p1, p2], p3)
cq_group("cq 2", [p1, p2], None)

# However, duplicate limit names within a cq_group is not allowed.
assert.fails(
    lambda: cq_group("cq 3", [p1, p1], None),
    "user_limits\\[1\\]: duplicate limit name 'user_limits'",
)
assert.fails(
    lambda: cq_group("cq 3", [p1, p2], cq.user_limit(name = p1.name)),
    "user_limit_default: limit name 'user_limits' is already used in user_limits",
)

# All the policies in `user_limits` must have a user or group.
assert.fails(
    lambda: cq_group("cq 3", [p3], None),
    "user_limits\\[0\\]: must specify at least one user or group",
)

# The limits in `user_limit_default` must not have a user and group.
assert.fails(
    lambda: cq_group("cq 3", [p1], p2),
    "user_limit_default: must not specify user or group",
)
