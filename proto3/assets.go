package proto3

import "embed"

//go:embed sillygirl.js sillygirl.d.ts srpc.js sillygirl.py srpc_pb2.py srpc_pb2_grpc.py
var runtimeFiles embed.FS

func ReadRuntimeFile(name string) ([]byte, error) {
	return runtimeFiles.ReadFile(name)
}
