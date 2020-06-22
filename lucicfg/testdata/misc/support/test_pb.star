load("@stdlib//internal/luci/descpb.star", "lucitypes_descpb")

# Add test.proto descriptor to the registry.
proto.new_descriptor_set(
    name = "test",
    blob = io.read_file("test_descpb.bin"),
    deps = [lucitypes_descpb],  # for "go.chromium.org/luci/common/proto/options.proto"
).register()

# Now that it is in the registry, load the proto module and export it publicly.
load("@proto//misc/support/test.proto", _testproto = "testproto")

testproto = _testproto
