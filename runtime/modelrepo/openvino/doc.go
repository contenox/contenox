// Package openvino is the runtime-side modelprovider for OpenVINO (Intel) local
// inference. It is pure Go: it assembles the workspace prompt into a stable
// prefix and volatile suffix, then drives the session
// (EnsurePrefix/PrefillSuffix/Decode) on the modeld daemon over runtime/transport
// via modeldconn. The CGO OpenVINO GenAI backend — tokenizer, chat template, and
// the ContinuousBatchingPipeline prefix cache — lives in modeld
// (modeld/openvino); inference availability is detected, not compiled in here.
package openvino
