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
    assert.eq(strutil.b64_encode("\u0001"), "AQ==")
    assert.eq(strutil.b64_decode("AQ=="), "\u0001")
    assert.fails(lambda: strutil.b64_decode("/w="), "illegal base64 data")
    assert.fails(lambda: strutil.b64_decode("_w=="), "illegal base64 data")

def test_hex_encoding():
    assert.eq(strutil.hex_encode(""), "")
    assert.eq(strutil.hex_decode(""), "")
    assert.eq(strutil.hex_encode("abcZ\u0000\u0001\u00ff"), "6162635a0001c3bf")
    assert.eq(strutil.hex_decode("6162635a0001c3bf"), "abcZ\u0000\u0001\u00ff")
    assert.eq(strutil.hex_decode("6162635A0001C3BF"), "abcZ\u0000\u0001\u00ff")
    assert.fails(lambda: strutil.hex_decode("huh"), "invalid byte")

test_expand_int_set()
test_json_to_yaml()
test_to_yaml()
test_b64_encoding()
test_hex_encoding()
