Hacks area
----------

`gensupport` directory here (excluding tests, add preserving the original
LICENSE) was copied verbatim from [google-api-go-client.git], since the original
package is in `internal` now and no longer importable from luci-go.

See [https://crbug.com/1003496] for more info.

This is **a temporary solution** until luci-go no longer depends on deprecated
Cloud Endpoints v1 APIs. Note that copied `gensupport` still depends on public
bits of `google-api-go-client.git`, so if  they drift apart too much, stuff
will break.

[google-api-go-client.git]: https://code.googlesource.com/google-api-go-client.git/+/master/internal/gensupport/
