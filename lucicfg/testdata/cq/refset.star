load('@stdlib//internal/luci/lib/cq.star', 'cq')


def test_refset_ok():
  def cmp(in_repo, in_refs, out_gob, out_proj, out_refs):
    refset = cq.refset(repo=in_repo, refs=in_refs)
    assert.eq(refset.__gob_host, out_gob)
    assert.eq(refset.__gob_proj, out_proj)
    assert.eq(refset.__refs, out_refs)
  cmp('https://example.googlesource.com/repo', ['.*'], 'example', 'repo', ['.*'])
  cmp('https://example-review.googlesource.com/a/zzz/repo.git', None, 'example', 'zzz/repo', ['refs/heads/master'])


def test_refset_fail():
  def fail(repo, refs, msg):
    assert.fails(lambda: cq.refset(repo=repo, refs=refs), msg)
  fail('example.googlesource.com/repo', None, 'should match "https://')
  fail('https://github.com/zzz', None, 'only \*\.googlesource.com repos are supported currently')
  fail('https://-review.googlesource.com/repo', None, 'not a valid repository URL')
  fail('https://example.googlesource.com/a/', None, 'not a valid repository URL')


test_refset_ok()
test_refset_fail()
