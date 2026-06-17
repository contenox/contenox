//go:build openvino && openvino_genai

package main

import (
	"github.com/contenox/runtime/modeld/openvino"
	"github.com/contenox/runtime/runtime/transport"
)

// selectBackend serves the OpenVINO GenAI backend (CPU / GPU / NPU, chosen by
// CONTENOX_OPENVINO_DEVICE) when built with the openvino tags.
func selectBackend() (transport.Service, string) { return &openvino.Service{}, "openvino" }
