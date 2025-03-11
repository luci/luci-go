load("@stdlib//internal/strutil.star", "strutil")

def test_expand_int_set():
    # Most tests are on the go side. Here we test only Starlark integration.
    assert.eq(strutil.expand_int_set("a{01..03}b"), ["a01b", "a02b", "a03b"])
    assert.fails(
        lambda: strutil.expand_int_set("}{"),
        'expand_int_set: bad expression - "}" must appear after "{"',
    )

def test_json_to_yaml():
    test = lambda i, o: assert.eq(strutil.json_to_yaml(i), o)
    test("", "null\n")
    test('"abc"', "abc\n")
    test('"abc: def"', "'abc: def'\n")
    test("123.0", "123\n")
    test("{}", "{}\n")
    test('{"abc": {"def": [1, "2", null, 3]}, "zzz": []}', """abc:
  def:
  - 1
  - "2"
  - null
  - 3
zzz: []
""")

def test_to_yaml():
    yaml = strutil.to_yaml({"a": ["b", "c"], "d": "huge\nhuge\nhuge\nhuge"})
    assert.eq("\n" + yaml, """
a:
- b
- c
d: |-
  huge
  huge
  huge
  huge
""")

def test_b64_encoding():
    # TODO(vadimsh): Use b"..." for binary data.
    assert.eq(strutil.b64_encode("\x01"), "AQ==")
    assert.eq(strutil.b64_decode("AQ=="), "\x01")
    assert.fails(lambda: strutil.b64_decode("/w="), "illegal base64 data")
    assert.fails(lambda: strutil.b64_decode("_w=="), "illegal base64 data")

def test_hex_encoding():
    assert.eq(strutil.hex_encode(""), "")
    assert.eq(strutil.hex_decode(""), "")
    assert.eq(strutil.hex_encode("abcZ\x00\x01\x1f"), "6162635a00011f")
    assert.eq(strutil.hex_decode("6162635a00011f"), "abcZ\x00\x01\x1f")
    assert.eq(strutil.hex_decode("6162635A00011F"), "abcZ\x00\x01\x1f")
    assert.fails(lambda: strutil.hex_decode("huh"), "invalid byte")

def test_parse_version():
    assert.eq(strutil.parse_version("1.2.3"), (1, 2, 3))
    assert.eq(strutil.parse_version("1.2"), (1, 2, 0))
    assert.eq(strutil.parse_version("1.2.3.4"), (1, 2, 3))
    assert.eq(strutil.parse_version("0.0.1"), (0, 0, 1))
    assert.eq(strutil.parse_version("0"), (0, 0, 0))
    assert.fails(lambda: strutil.parse_version(None), "string is required")
    assert.fails(lambda: strutil.parse_version(123), "want string")
    assert.fails(lambda: strutil.parse_version(""), "bad version string")
    assert.fails(lambda: strutil.parse_version("a"), "bad version string")
    assert.fails(lambda: strutil.parse_version("1."), "bad version string")
    assert.fails(lambda: strutil.parse_version("1..1"), "bad version string")
    assert.fails(lambda: strutil.parse_version("-1.0"), "bad version string")
    assert.fails(lambda: strutil.parse_version("1.0.1-rc1"), "bad version string")

test_expand_int_set()
test_json_to_yaml()
test_to_yaml()
test_b64_encoding()
test_hex_encoding()
test_parse_version()
