luci.project(name = "foo")

p1 = cq.quota_policy(name = "user_policy", users = ["someone@example.com"])
p2 = cq.quota_policy(name = "group_policy", groups = ["abc"])
p3 = cq.quota_policy(name = "no_principal_policy")

def cq_group(name, user_quotas, user_quotas_default):
    luci.cq_group(
        name = name,
        watch = [cq.refset("https://example.googlesource.com/proj/repo")],
        acls = [acl.entry(acl.CQ_COMMITTER, groups = ["committers"])],
        user_quotas = user_quotas,
        user_quota_default = user_quotas_default,
    )

# cq.quota_policy(s) can be passed to multiple cq_group(s),
# However, it doesn't mean that the quota balances will be shared across
# the cq groups. User quotas are tracked seperately for each cq_group.
cq_group("cq 1", [p1, p2], p3)
cq_group("cq 2", [p1, p2], None)

# However, duplicate policy names within a cq_group is not allowed.
assert.fails(
    lambda: cq_group("cq 3", [p1, p1], None),
    "user_quotas\\[1\\]: duplicate policy name 'user_policy'",
)
assert.fails(
    lambda: cq_group("cq 3", [p1, p2], cq.quota_policy(name = p1.name)),
    "user_quota_default: policy 'user_policy' is already used in user_quotas",
)

# All the policies in `user_quotas` must have a user or group.
assert.fails(
    lambda: cq_group("cq 3", [p3], None),
    "user_quotas\\[0\\]: must specify at least one user or group",
)

# The policy in `user_quota_default` must not have a user and group.
assert.fails(
    lambda: cq_group("cq 3", [p1], p2),
    "user_quota_default: must not specify user or group",
)
