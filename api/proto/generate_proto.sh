protoc \
  --proto_path=api/proto \
  --go_out=api/proto \
  --go_opt=paths=source_relative \
  api/proto/memorydb.proto