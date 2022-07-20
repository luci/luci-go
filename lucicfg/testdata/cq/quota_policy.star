# good names
cq.quota_policy(name = "this_is_a_good_name", users = ["foo@example.com"])
cq.quota_policy(name = "policy-abc@example.com", groups = ["engineers"])

# bad names
def fail(name, msg):
    assert.fails(lambda: cq.quota_policy(name = name), msg)

fail("", "must not be empty")
fail("@a", "should match \"\\^\\[")
fail("Abc Def", "should match \"\\^\\[")
