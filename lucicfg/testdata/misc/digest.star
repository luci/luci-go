load("@stdlib//internal/digest.star", "digest")

def test_sha256():
    null = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
    assert.eq(digest.sha256.hex(""), null)
    assert.eq(digest.sha256.size * 2, len(null))
    abc = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
    assert.eq(digest.sha256.hex("abc"), abc)

test_sha256()
