# astpb

Generated Protocol Buffers message types for ByteDance / Volcengine's Audio
Speech Technology (AST) real-time understanding protocol. These types are used
by the ByteDance interpreting adapter to frame messages over the WebSocket
transport (see `internal/provider/bytedance/interpreting.go`).

## Contents

- `products/understanding/ast/ast_service.pb.go` — message types.
- `common/**` — shared message types referenced by the AST messages.

Only the Protocol Buffers **message** types are used. The generated gRPC service
client/server stub (`ast_service_grpc.pb.go`) was removed because Polaris speaks
this protocol over its own WebSocket transport, not gRPC; it pulled in the gRPC
runtime for no functional reason.

## Provenance & regeneration

The `.proto` sources are ByteDance/Volcengine's published AST definitions. Only
the message descriptors are vendored here. If the upstream protocol changes,
regenerate the message types with `protoc` (Go plugin) against the upstream
`.proto` files and re-vendor only the message `.pb.go` outputs — do not
re-introduce the gRPC service stub.
