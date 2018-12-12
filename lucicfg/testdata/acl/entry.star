load('@stdlib//internal/luci/lib/acl.star', 'acl')


def check_entry(entry, roles, groups=[], users=[]):
  assert.eq(entry.roles, roles)
  assert.eq(entry.groups, groups)
  assert.eq(entry.users, users)


def test_roles_validation():
  # Entry without users or groups is OK.
  check_entry(
      entry = acl.entry(acl.BUILDBUCKET_READER),
      roles = [acl.BUILDBUCKET_READER],
  )
  # Many roles is OK.
  check_entry(
      entry = acl.entry([acl.BUILDBUCKET_READER, acl.BUILDBUCKET_SCHEDULER]),
      roles = [acl.BUILDBUCKET_READER, acl.BUILDBUCKET_SCHEDULER],
  )
  # No roles is NOT ok.
  assert.fails(lambda: acl.entry([]), 'bad "roles": missing')
  assert.fails(lambda: acl.entry(None), 'bad "roles": missing')
  assert.fails(lambda: acl.entry([None]), 'bad "roles": missing')
  # Invalid type is NOT ok.
  assert.fails(
      lambda: acl.entry('zzz'),
      'bad "roles": got string "zzz", expecting acl.role')
  assert.fails(
      lambda: acl.entry(['zzz']),
      'bad "roles": got string "zzz", expecting acl.role')


def test_groups_validation():
  # Singular group is OK.
  check_entry(
      entry = acl.entry(acl.BUILDBUCKET_READER, groups='grr'),
      roles = [acl.BUILDBUCKET_READER],
      groups = ['grr'],
  )
  # Multiple groups is OK.
  check_entry(
      entry = acl.entry(acl.BUILDBUCKET_READER, groups=['grr1', 'grr2']),
      roles = [acl.BUILDBUCKET_READER],
      groups = ['grr1', 'grr2'],
  )
  # Empty list is OK.
  check_entry(
      entry = acl.entry(acl.BUILDBUCKET_READER, groups=[]),
      roles = [acl.BUILDBUCKET_READER],
      groups = [],
  )
  # Wrong type is not OK.
  assert.fails(
      lambda: acl.entry(acl.BUILDBUCKET_READER, groups=123),
      'bad "groups": got int 123, expecting string')


def test_users_validation():
  # Singular user is OK.
  check_entry(
      entry = acl.entry(acl.BUILDBUCKET_READER, users='a@example.com'),
      roles = [acl.BUILDBUCKET_READER],
      users = ['a@example.com'],
  )
  # Multiple users is OK.
  check_entry(
      entry = acl.entry(
          acl.BUILDBUCKET_READER,
          users = ['a@example.com', 'b@example.com']),
      roles = [acl.BUILDBUCKET_READER],
      users = ['a@example.com', 'b@example.com'],
  )
  # Empty list is OK.
  check_entry(
      entry = acl.entry(acl.BUILDBUCKET_READER, users=[]),
      roles = [acl.BUILDBUCKET_READER],
      users = [],
  )
  # Wrong type is not OK.
  assert.fails(
      lambda: acl.entry(acl.BUILDBUCKET_READER, users=123),
      'bad "users": got int 123, expecting string')


def test_group_only_roles():
  assert.true(acl.LOGDOG_READER.groups_only)
  # Works with groups.
  check_entry(
      entry = acl.entry(acl.LOGDOG_READER, groups='g'),
      roles = [acl.LOGDOG_READER],
      groups = ['g'],
  )
  # Fails with users.
  assert.fails(
      lambda: acl.entry(acl.LOGDOG_READER, users='a@example.com'),
      'role LOGDOG_READER can be assigned only to groups, not individual users')


test_roles_validation()
test_groups_validation()
test_users_validation()
test_group_only_roles()
