def test_read_file_ok():
  assert.eq(io.read_file('support/file_to_read.txt'), 'Hello, world!\n')
  assert.eq(io.read_file('//testdata/misc/support/file_to_read.txt'), 'Hello, world!\n')

def test_read_file_fail():
  assert.fails(lambda: io.read_file('missing_file.txt'), 'no such file')
  assert.fails(lambda: io.read_file('//missing_file.txt'), 'no such file')
  assert.fails(lambda: io.read_file('../../../README.md'), 'outside the package root')

test_read_file_ok()
test_read_file_fail()
