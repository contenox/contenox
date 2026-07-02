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

# The installed OpenVINO/GenAI shared libraries carry a version-encoded SONAME
# (e.g. libopenvino.so.2621) that changes with every OpenVINO patch release,
# and `pip install openvino[-genai]` above is unpinned, so the exact patch pip
# resolves can drift from OPENVINO_GENAI_VERSION. `ld -l:<exact-file>` requires
# a literal filename match, so resolve the SONAME from what is actually on
# disk instead of hardcoding it — a hardcoded suffix silently breaks the link
# step the moment a newer/older patch release installs a different SONAME.
OPENVINO_SO := $(notdir $(firstword $(wildcard $(OPENVINO_PKG)/libs/libopenvino.so.[0-9]*)))
OPENVINO_GENAI_SO := $(notdir $(firstword $(wildcard $(OPENVINO_GENAI_PKG)/libopenvino_genai.so.[0-9]*)))

OPENVINO_CGO_LDFLAGS = -L$(OPENVINO_PKG)/libs -l:$(OPENVINO_SO) -lstdc++ $(OPENVINO_RPATH_DTAGS) -Wl,-rpath,$(OPENVINO_PKG)/libs
OPENVINO_GENAI_CGO_CXXFLAGS = $(OPENVINO_CGO_CXXFLAGS) -I$(OPENVINO_GENAI_SRC)/src/cpp/include -I$(OPENVINO_GENAI_SRC)/src/cpp/src -I$(OPENVINO_GENAI_SRC)/build/_deps/minja-src/include -I$(OPENVINO_GENAI_SRC)/build/_deps/gguflib-src

# GenAI link flags without a runtime rpath.
# Tests and packages append the appropriate runtime rpath.
OPENVINO_GENAI_LINK_FLAGS = -L$(OPENVINO_PKG)/libs -L$(OPENVINO_GENAI_PKG) -L$(OPENVINO_TOKENIZERS_LIB) -l:$(OPENVINO_GENAI_SO) -l:$(OPENVINO_SO) -lstdc++ $(OPENVINO_RPATH_DTAGS) -Wl,-rpath-link,$(OPENVINO_PKG)/libs -Wl,-rpath-link,$(OPENVINO_GENAI_PKG) -Wl,-rpath-link,$(OPENVINO_TOKENIZERS_LIB)
OPENVINO_GENAI_CGO_LDFLAGS = $(OPENVINO_GENAI_LINK_FLAGS) -Wl,-rpath,$(OPENVINO_PKG)/libs -Wl,-rpath,$(OPENVINO_GENAI_PKG) -Wl,-rpath,$(OPENVINO_TOKENIZERS_LIB)
