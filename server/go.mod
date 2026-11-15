module github.com/pescador95/memorydb/server

go 1.27.0

require (
    github.com/pescador95/memorydb/protocol/generated/go v0.0.0
    google.golang.org/protobuf v1.36.12
)

replace github.com/pescador95/memorydb/protocol/generated/go => ../protocol/generated/go