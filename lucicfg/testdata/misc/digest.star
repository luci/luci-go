load('@stdlib//internal/digest.star', 'digest')


def test_sha256():
  null = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'
  assert.eq(digest.hex(digest.SHA256, ''), null)
  assert.eq(digest.size(digest.SHA256)*2, len(null))
  abc = 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad'
  assert.eq(digest.hex(digest.SHA256, 'abc'), abc)


def test_unknown_algo():
  assert.fails(lambda: digest.size('zzz'), 'unknown hashing algorithm "zzz"')
  assert.fails(lambda: digest.bytes('zzz', ''), 'unknown hashing algorithm "zzz"')
  assert.fails(lambda: digest.hex('zzz', ''), 'unknown hashing algorithm "zzz"')


test_sha256()
test_unknown_algo()
