// Package openvino contains the modelrepo catalog/provider shell for in-process
// OpenVINO (Intel) inference.
//
// The actual native backend lives in the isolated sub-package ./ovsession and is
// gated behind the 'openvino' build tag, so the default build and CI never
// require OpenVINO or a C++ toolchain. Without the tag, ovsession reports
// Available == false and this provider advertises no models.
//
// The native session layer currently proves token-ID-level KV snapshot/restore.
// Text chat, prompt, stream, embedding, tokenizer, and chat-template wiring are
// still explicit follow-up work.
//
// Build and test the native backend with Makefile.openvino:
//
//	make -f Makefile.openvino deps   # one-time: venv + OpenVINO SDK
//	make -f Makefile.openvino model  # pull the tiny KV round-trip model
//	make -f Makefile.openvino test   # build + run the S0 KV snapshot test
package openvino
