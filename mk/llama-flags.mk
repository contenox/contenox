# Shared llama.cpp build configuration: the vendored single-header tree and the
# pinned llama.cpp commit. Included by Makefile.llamacpp (vendor + test targets)
# and the top-level Makefile (build-modeld) so the vendor path and the pin are a
# single source of truth and cannot drift.
#
# Requires PROJECT_ROOT (with trailing slash) from the including Makefile.

# The ollama Go module strips llama.cpp's vendored single-header deps
# (nlohmann/json, miniaudio, stb), so CGO can't compile it as-is. We fetch them
# here and feed via CGO_CPPFLAGS=-I$(LLAMA_VENDOR). minja (the model-native Jinja
# chat-template engine) is pinned to the SAME commit ollama vendors, so the
# template engine matches the linked llama.cpp.
LLAMA_VENDOR ?= $(PROJECT_ROOT).llamacpp-vendor
LLAMA_CPP_COMMIT ?= ec98e2002
