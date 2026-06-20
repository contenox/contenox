# Shared OpenVINO build configuration.
# Requires PROJECT_ROOT from the including Makefile.

OPENVINO_WORKDIR ?= $(PROJECT_ROOT).openvino
OPENVINO_VENV ?= $(OPENVINO_WORKDIR)/venv
OPENVINO_GENAI_VERSION ?= 2026.2.0.0
OPENVINO_GENAI_SRC ?= $(HOME)/src/github.com/openvinotoolkit/openvino.genai-$(OPENVINO_GENAI_VERSION)
PYTHON ?= python3

OPENVINO_PKG = $(shell test -x "$(OPENVINO_VENV)/bin/python" && "$(OPENVINO_VENV)/bin/python" -c 'import openvino, os; print(os.path.dirname(openvino.__file__))' 2>/dev/null)
OPENVINO_GENAI_PKG = $(shell test -x "$(OPENVINO_VENV)/bin/python" && "$(OPENVINO_VENV)/bin/python" -c 'import openvino_genai, os; print(os.path.dirname(openvino_genai.__file__))' 2>/dev/null)
OPENVINO_TOKENIZERS_PKG = $(shell test -x "$(OPENVINO_VENV)/bin/python" && "$(OPENVINO_VENV)/bin/python" -c 'import openvino_tokenizers, os; print(os.path.dirname(openvino_tokenizers.__file__))' 2>/dev/null)
OPENVINO_TOKENIZERS_LIB = $(OPENVINO_TOKENIZERS_PKG)/lib
OPENVINO_TOKENIZERS_SO = $(OPENVINO_TOKENIZERS_LIB)/libopenvino_tokenizers.so
OPENVINO_CGO_CXXFLAGS = -std=c++17 -I$(OPENVINO_PKG)/include
# Use DT_RPATH so OpenVINO transitive libraries resolve without LD_LIBRARY_PATH.
OPENVINO_RPATH_DTAGS = -Wl,--disable-new-dtags
OPENVINO_CGO_LDFLAGS = -L$(OPENVINO_PKG)/libs -l:libopenvino.so.2620 -lstdc++ $(OPENVINO_RPATH_DTAGS) -Wl,-rpath,$(OPENVINO_PKG)/libs
OPENVINO_GENAI_CGO_CXXFLAGS = $(OPENVINO_CGO_CXXFLAGS) -I$(OPENVINO_GENAI_SRC)/src/cpp/include

# GenAI link flags without a runtime rpath.
# Tests and packages append the appropriate runtime rpath.
OPENVINO_GENAI_LINK_FLAGS = -L$(OPENVINO_PKG)/libs -L$(OPENVINO_GENAI_PKG) -L$(OPENVINO_TOKENIZERS_LIB) -l:libopenvino_genai.so.2620 -l:libopenvino.so.2620 -lstdc++ $(OPENVINO_RPATH_DTAGS) -Wl,-rpath-link,$(OPENVINO_PKG)/libs -Wl,-rpath-link,$(OPENVINO_GENAI_PKG) -Wl,-rpath-link,$(OPENVINO_TOKENIZERS_LIB)
OPENVINO_GENAI_CGO_LDFLAGS = $(OPENVINO_GENAI_LINK_FLAGS) -Wl,-rpath,$(OPENVINO_PKG)/libs -Wl,-rpath,$(OPENVINO_GENAI_PKG) -Wl,-rpath,$(OPENVINO_TOKENIZERS_LIB)
