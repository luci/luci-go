-- `Properties` is a serialized then compressed google.protobuf.Struct that
-- stores structured, domain-specific properties of the invocation.
ALTER TABLE Invocations ADD COLUMN Properties BYTES(MAX);

-- `Properties` is a serialized then compressed google.protobuf.Struct that
-- stores structured, domain-specific properties of the test result.
ALTER TABLE TestResults ADD COLUMN Properties BYTES(MAX);
