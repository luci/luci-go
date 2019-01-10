load('@stdlib//internal/luci/lib/acl.star', 'acl', 'aclimpl')


def test_works():
  # Empty list is fine.
  assert.eq(aclimpl.normalize_acls([]), [])

  # Sorts and dedups.
  ents = [
      acl.entry(acl.BUILDBUCKET_OWNER, groups=['group a', 'group b'], users='a@example.com'),
      acl.entry(acl.BUILDBUCKET_READER, users='b@example.com'),
      acl.entry(acl.BUILDBUCKET_READER, users='a@example.com'),
      acl.entry(acl.BUILDBUCKET_READER, users=['a@example.com', 'c@example.com']),
      acl.entry(acl.BUILDBUCKET_READER),  # should be ignored
      acl.entry(acl.BUILDBUCKET_READER, groups='group b'),
      acl.entry(acl.BUILDBUCKET_READER, groups='group a'),
  ]
  assert.eq([(e.role, e.user, e.group) for e in aclimpl.normalize_acls(ents)], [
      (acl.BUILDBUCKET_OWNER, None, 'group a'),
      (acl.BUILDBUCKET_OWNER, None, 'group b'),
      (acl.BUILDBUCKET_OWNER, 'a@example.com', None),
      (acl.BUILDBUCKET_READER, None, 'group a'),
      (acl.BUILDBUCKET_READER, None, 'group b'),
      (acl.BUILDBUCKET_READER, 'a@example.com', None),
      (acl.BUILDBUCKET_READER, 'b@example.com', None),
      (acl.BUILDBUCKET_READER, 'c@example.com', None),
  ])


test_works()
