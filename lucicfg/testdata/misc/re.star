load("@stdlib//internal/re.star", "re")

RE = r"^(([[:graph:]]+)/)?([[:graph:]]+)$"

def test():
    groups = re.submatches(RE, "abc/def")
    assert.eq(groups[-2:], ("abc", "def"))

    groups = re.submatches(RE, "abc")
    assert.eq(groups[-2:], ("", "abc"))

    groups = re.submatches(RE, "")
    assert.true(not groups)

test()
