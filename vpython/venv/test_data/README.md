## Test Data?

This test data directory is used by VirtualEnv tests to simulate a full
environment setup.

A fake CIPD client in `venv_resources_test.go` will use this to generate a fake
CIPD archive.

Wheels are generated from source that is also checked into the `test_data`. The
source for a wheel, "foo", is located at `foo.src`.

To build a wheel from source, `cd` into a source directory and run:

    $ python setup.py bdist_wheel

The wheel will be created in `/dist/`.
