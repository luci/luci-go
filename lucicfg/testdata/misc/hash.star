load('@stdlib//internal/hash.star', 'hash')


def test_sha256():
  null = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'
  assert.eq(hash.hexdigest(hash.SHA256, ''), null)
  assert.eq(hash.size(hash.SHA256)*2, len(null))
  abc = 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad'
  assert.eq(hash.hexdigest(hash.SHA256, 'abc'), abc)


def test_unknown_algo():
  assert.fails(lambda: hash.size('zzz'), 'unknown hashing algorithm "zzz"')
  assert.fails(lambda: hash.digest('zzz', ''), 'unknown hashing algorithm "zzz"')
  assert.fails(lambda: hash.hexdigest('zzz', ''), 'unknown hashing algorithm "zzz"')


test_sha256()
test_unknown_algo()
