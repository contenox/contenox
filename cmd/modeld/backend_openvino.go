//go:build openvino && openvino_genai

package main

import (
	"github.com/contenox/runtime/modeld/openvino"
	"github.com/contenox/runtime/runtime/transport"
)

// Register the OpenVINO GenAI backend (CPU / GPU / NPU, chosen by
// CONTENOX_OPENVINO_DEVICE); selectBackend (backend.go) serves it when it is the
// only one compiled in or when CONTENOX_MODELD_BACKEND=openvino.
func init() { registerBackend("openvino", func() transport.Service { return &openvino.Service{} }) }
