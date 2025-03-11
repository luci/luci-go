def test_lucicfg_version():
    ver = lucicfg.version()
    assert.eq(type(ver), "tuple")
    assert.eq(len(ver), 3)
    for i in ver:
        assert.eq(type(i), "int")
    assert.true(ver >= (1, 0, 0))

def test_check_version():
    major, minor, rev = lucicfg.version()

    lucicfg.check_version(min = "%s.%s.%s" % (major, minor, rev))
    lucicfg.check_version(min = "%s.%s" % (major, minor))
    lucicfg.check_version(min = "%s.%s.%s" % (major, minor - 1, rev))

    assert.fails(
        lambda: lucicfg.check_version(min = "%s.%s.%s" % (major, minor, rev + 1)),
        r"Your lucicfg version v([0-9]|\.)+ is older than required v([0-9]|\.)+\. Please update\.",
    )

test_lucicfg_version()
test_check_version()
