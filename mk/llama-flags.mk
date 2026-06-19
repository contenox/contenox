# Shared llama.cpp build configuration: the pinned upstream source checkout and
# the generated direct runtime location. Included by Makefile.llamacpp-direct
# (native library build) and the top-level Makefile (modeld build) so common chat
# headers and the linked llama.cpp runtime cannot drift.
#
# Requires PROJECT_ROOT (with trailing slash) from the including Makefile.

LLAMA_CPP_COMMIT ?= ee3a5a10adf9e83722d1914dddc56a0623ececaf

# Upstream reference source for the direct llama.cpp runtime build. This checkout
# is intentionally ignored under tmp/; LLAMA_CPP_COMMIT is the production pin.
LLAMA_CPP_REF_REPO ?= https://github.com/ggml-org/llama.cpp.git
LLAMA_CPP_REF_DIR ?= $(PROJECT_ROOT)tmp/ref/llama.cpp

# Pinned direct llama.cpp runtime build output. Profiles are directories under
# this root, e.g. cpu and cuda. These are generated artifacts, not source.
LLAMA_RUNTIME_ROOT ?= $(PROJECT_ROOT).llamacpp-runtime
LLAMA_RUNTIME_PROFILE ?= cpu
LLAMA_RUNTIME_DIR ?= $(LLAMA_RUNTIME_ROOT)/$(LLAMA_RUNTIME_PROFILE)
LLAMA_RUNTIME_LIB_DIR ?= $(LLAMA_RUNTIME_DIR)/lib
LLAMA_DIRECT_CPPFLAGS = -I$(LLAMA_RUNTIME_DIR)/include
LLAMA_COMMON_CPPFLAGS = -I$(LLAMA_CPP_REF_DIR)/common -I$(LLAMA_CPP_REF_DIR)/vendor
LLAMA_DIRECT_BACKEND_LIBS = -l:libggml-cpu.so
ifeq ($(LLAMA_RUNTIME_PROFILE),cuda)
LLAMA_DIRECT_BACKEND_LIBS += -l:libggml-cuda.so
endif
LLAMA_DIRECT_LINK_LIBS = -l:libcommon.a -l:libllama.so -l:libggml.so -l:libggml-base.so $(LLAMA_DIRECT_BACKEND_LIBS) -lstdc++ -lm -ldl -lpthread
LLAMA_DIRECT_LDFLAGS = -L$(LLAMA_RUNTIME_LIB_DIR) -Wl,--disable-new-dtags -Wl,-rpath,$(LLAMA_RUNTIME_LIB_DIR) -Wl,-rpath-link,$(LLAMA_RUNTIME_LIB_DIR) $(LLAMA_DIRECT_LINK_LIBS)
