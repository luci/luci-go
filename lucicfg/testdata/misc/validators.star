load("@stdlib//internal/time.star", "time")
load("@stdlib//internal/validate.star", "validate")

def test_validate_string():
    call = validate.string
    assert.eq(call("a", "zzz"), "zzz")
    assert.eq(call("a", "abc", regexp = "a.c$"), "abc")
    assert.eq(call("a", "", default = "zzz", allow_empty = True, required = False), "")
    assert.eq(call("a", None, default = "zzz", allow_empty = True, required = False), "zzz")
    assert.eq(call("a", None, required = False), None)
    assert.fails(lambda: call("a", None), 'missing required field "a"')
    assert.fails(lambda: call("a", 1), 'bad "a": got int, want string')
    assert.fails(lambda: call("a", []), 'bad "a": got list, want string')
    assert.fails(lambda: call("a", None, default = 1, required = False), 'bad "a": got int, want string')
    assert.fails(lambda: call("a", "abcd", regexp = "a.c$"), r'bad "a": "abcd" should match "a\.c\$"')
    assert.fails(lambda: call("a", "", default = "zzz", required = False), 'bad "a": must not be empty')

def test_validate_hostname():
    call = validate.hostname
    assert.eq(call("a", "zzz"), "zzz")
    assert.eq(call("a", "test-domain.example.com"), "test-domain.example.com")
    assert.eq(call("a", "xn--ls8h.example.com"), "xn--ls8h.example.com")
    assert.eq(call("a", None, default = "zzz", required = False), "zzz")
    assert.eq(call("a", None, required = False), None)

    assert.fails(lambda: call("a", None), 'missing required field "a"')
    assert.fails(lambda: call("a", 1), 'bad "a": got int, want string')
    assert.fails(lambda: call("a", []), 'bad "a": got list, want string')
    assert.fails(lambda: call("a", None, default = 1, required = False), 'bad "a": got int, want string')
    assert.fails(lambda: call("a", "!notdomain"), r'bad "a": "!notdomain" is not valid RFC1123 hostname')
    assert.fails(
        lambda: call("a", "https://i.am.a.url.example.com"),
        r'bad "a": "https://i.am.a.url.example.com" is not valid RFC1123 hostname',
    )
    assert.fails(lambda: call("a", "", default = "zzz", required = False), 'bad "a": must not be empty')

def test_validate_int():
    call = validate.int
    assert.eq(call("a", 123, min = 123, max = 123), 123)
    assert.eq(call("a", 0, default = 123, required = False), 0)
    assert.eq(call("a", None, default = 123, required = False), 123)
    assert.eq(call("a", None, required = False), None)
    assert.fails(lambda: call("a", None), 'missing required field "a"')
    assert.fails(lambda: call("a", "1"), 'bad "a": got string, want int')
    assert.fails(lambda: call("a", []), 'bad "a": got list, want int')
    assert.fails(lambda: call("a", None, default = "1", required = False), 'bad "a": got string, want int')
    assert.fails(lambda: call("a", 123, min = 124), 'bad "a": 123 should be >= 124')
    assert.fails(lambda: call("a", 123, max = 122), 'bad "a": 123 should be <= 122')

def test_validate_float():
    call = validate.float
    assert.eq(call("a", 123.4, min = 123, max = 123.5), 123.4)
    assert.eq(call("a", 123, min = 123, max = 123), 123.0)
    assert.eq(call("a", 0, default = 123, required = False), 0.0)
    assert.eq(call("a", None, default = 123, required = False), 123.0)
    assert.eq(call("a", None, required = False), None)
    assert.fails(lambda: call("a", None), 'missing required field "a"')
    assert.fails(lambda: call("a", "1"), 'bad "a": got string, want float or int')
    assert.fails(lambda: call("a", []), 'bad "a": got list, want float or int')
    assert.fails(lambda: call("a", None, default = "1", required = False), 'bad "a": got string, want float or int')
    assert.fails(lambda: call("a", 123, min = 124.5), 'bad "a": 123.0 should be >= 124.5')
    assert.fails(lambda: call("a", 123, max = 122.5), 'bad "a": 123.0 should be <= 122.5')

def test_validate_bool():
    call = validate.bool
    assert.eq(call("a", True), True)
    assert.eq(call("a", 1), True)
    assert.eq(call("a", [1]), True)
    assert.eq(call("a", []), False)
    assert.eq(call("a", None, default = True, required = False), True)
    assert.eq(call("a", None, default = False, required = False), False)
    assert.eq(call("a", None, required = False), None)
    assert.fails(lambda: call("a", None), 'missing required field "a"')

def test_validate_duration():
    call = validate.duration
    m = time.minute
    assert.eq(call("a", m), m)
    assert.eq(call("a", None, default = m, required = False), m)
    assert.eq(call("a", None, required = False), None)
    assert.fails(lambda: call("a", None), 'missing required field "a"')
    assert.fails(lambda: call("a", 1), 'bad "a": got int, want duration')
    assert.fails(lambda: call("a", []), 'bad "a": got list, want duration')
    assert.fails(lambda: call("a", -m), 'bad "a": -1m0s should be >= 0s')
    assert.fails(lambda: call("a", 2 * m, max = m), 'bad "a": 2m0s should be <= 1m0s')
    assert.fails(
        lambda: call("a", 500 * time.millisecond),
        'bad "a": losing precision when truncating 500ms to 1s units, use ' +
        r"time.truncate\(\.\.\.\) to acknowledge",
    )

def test_validate_list():
    call = validate.list
    assert.eq(call("a", [1, "a"]), [1, "a"])
    assert.eq(call("a", []), [])
    assert.eq(call("a", None), [])
    assert.fails(lambda: call("a", 0), 'bad "a": got int, want list')
    assert.fails(lambda: call("a", {}), 'bad "a": got dict, want list')
    assert.fails(lambda: call("a", None, required = True), 'missing required field "a"')
    assert.fails(lambda: call("a", [], required = True), 'missing required field "a"')

def test_validate_str_dict():
    call = validate.str_dict
    assert.eq(call("a", {"k": 1}), {"k": 1})
    assert.eq(call("a", {}), {})
    assert.eq(call("a", None), {})
    assert.fails(lambda: call("a", 0), 'bad "a": got int, want dict')
    assert.fails(lambda: call("a", []), 'bad "a": got list, want dict')
    assert.fails(lambda: call("a", {1: 1}), 'bad "a": got int key, want string')
    assert.fails(lambda: call("a", {"": 1}), 'bad "a": got empty key')
    assert.fails(lambda: call("a", None, required = True), 'missing required field "a"')
    assert.fails(lambda: call("a", {}, required = True), 'missing required field "a"')

def test_validate_struct():
    call = validate.struct
    s = struct(a = 1, b = 2)
    assert.eq(call("a", s, "struct"), s)
    assert.eq(call("a", None, "struct", default = s, required = False), s)
    assert.eq(call("a", None, "struct", required = False), None)
    assert.fails(lambda: call("a", None, "struct"), 'missing required field "a"')
    assert.fails(lambda: call("a", 1, "struct"), 'bad "a": got int, want struct')
    assert.fails(lambda: call("a", [], "struct"), 'bad "a": got list, want struct')
    assert.fails(lambda: call("a", None, "struct", default = 1, required = False), 'bad "a": got int, want struct')

def test_validate_type():
    call = validate.type
    assert.eq(call("a", 1, 0), 1)
    assert.eq(call("a", None, 0, default = 123, required = False), 123)
    assert.eq(call("a", None, 0, required = False), None)
    assert.fails(lambda: call("a", None, 0), 'missing required field "a"')
    assert.fails(lambda: call("a", "sss", 0), 'bad "a": got string, want int')
    assert.fails(lambda: call("a", [], 0), 'bad "a": got list, want int')

def test_validate_relative_path():
    call = validate.relative_path
    assert.eq(call("a", "./abc/def//./zzz/../yyy/."), "abc/def/yyy")
    assert.eq(call("a", "."), ".")
    assert.eq(call("a", "./../abc", allow_dots = True), "../abc")
    assert.eq(call("a", None, required = False), None)
    assert.fails(lambda: call("a", None), 'missing required field "a"')
    assert.fails(lambda: call("a", "./../abc"), 'must not start with "../"')
    assert.fails(lambda: call("a", "a/../../abc"), 'must not start with "../"')

    # Rebasing.
    assert.eq(call("a", "./abc", base = "./a/b"), "a/b/abc")
    assert.eq(call("a", "../abc", base = "./a/b"), "a/abc")
    assert.eq(call("a", "../../abc", base = "./a/b"), "abc")
    assert.eq(call("a", "../../../abc", base = "./a/b", allow_dots = True), "../abc")
    assert.fails(
        lambda: call("a", "../../../abc", base = "./a/b", allow_dots = False),
        'must not start with "../"',
    )
    assert.fails(
        lambda: call("a", "a/../../../../abc", base = "./a/b", allow_dots = False),
        'must not start with "../"',
    )

def test_validate_regex_list():
    call = validate.regex_list

    assert.fails(lambda: call("a", 123), 'bad "a": got int, want string or list')
    assert.fails(lambda: call("a", [123]), 'bad "a": got list element of type int, want string')
    assert.fails(lambda: call("a", None, required = True), 'missing required field "a"')
    assert.fails(lambda: call("a", "", required = True), 'missing required field "a"')
    assert.fails(lambda: call("a", [], required = True), 'missing required field "a"')
    assert.fails(lambda: call("a", "("), 'bad "a": error parsing regexp: missing closing \\): `\\(`')
    assert.fails(lambda: call("a", ["(ab", "bc)"]), 'bad "a": error parsing regexp: missing closing \\): `\\(ab`')

    assert.eq(call("a", ""), "")
    assert.eq(call("a", []), "")
    assert.eq(call("a", "abc.*def"), "abc.*def")
    assert.eq(call("a", "abcdef", required = True), "abcdef")
    assert.eq(call("a", ["xyz", "abc"]), "xyz|abc")
    assert.eq(call("a", ["xyz", "abc"], required = True), "xyz|abc")
    assert.eq(call("a", ["x.*yz", "defg", "abc"]), "x.*yz|defg|abc")

def test_validate_str_list():
    call = validate.str_list
    assert.eq(call("a", ["a", "b"]), ["a", "b"])
    assert.eq(call("a", []), [])
    assert.eq(call("a", None), [])
    assert.fails(lambda: call("a", 0), 'bad "a": got int, want list')
    assert.fails(lambda: call("a", {}), 'bad "a": got dict, want list')
    assert.fails(lambda: call("a", [100]), 'bad "a": got list element of type int, want string')
    assert.fails(lambda: call("a", None, required = True), 'missing required field "a"')
    assert.fails(lambda: call("a", [], required = True), 'missing required field "a"')

test_validate_string()
test_validate_hostname()
test_validate_int()
test_validate_float()
test_validate_bool()
test_validate_duration()
test_validate_list()
test_validate_str_dict()
test_validate_struct()
test_validate_type()
test_validate_relative_path()
test_validate_regex_list()
test_validate_str_list()
