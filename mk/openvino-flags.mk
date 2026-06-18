# Shared OpenVINO build configuration: paths into the .openvino venv plus the
# CGO flag sets for the SDK and GenAI C++ APIs. Included by Makefile.openvino
# (test targets) and the top-level Makefile (build-modeld) so the OpenVINO test
# build and the modeld release build resolve identical flags and cannot drift.
#
# Requires PROJECT_ROOT (with trailing slash) from the including Makefile.
# Every flag var is lazily expanded (=), so the python introspection only runs
# for targets that actually build the OpenVINO backend — including this fragment
# costs nothing for targets that don't.

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
# --disable-new-dtags makes the -rpath entries a DT_RPATH (not DT_RUNPATH).
# RUNPATH is NOT searched for transitively-loaded libraries, so libopenvino's own
# deps (libtbb.so.12, etc., which live in openvino/libs) fail to load when the
# binary is run without LD_LIBRARY_PATH. DT_RPATH is searched transitively, so the
# daemon is self-contained — `./bin/modeld serve` works with no env.
OPENVINO_RPATH_DTAGS = -Wl,--disable-new-dtags
OPENVINO_CGO_LDFLAGS = -L$(OPENVINO_PKG)/libs -l:libopenvino.so.2620 -lstdc++ $(OPENVINO_RPATH_DTAGS) -Wl,-rpath,$(OPENVINO_PKG)/libs
OPENVINO_GENAI_CGO_CXXFLAGS = $(OPENVINO_CGO_CXXFLAGS) -I$(OPENVINO_GENAI_SRC)/src/cpp/include

# Link flags (compiler search paths + libs), WITHOUT a runtime rpath. The OpenVINO
# test targets append absolute venv rpaths (OPENVINO_GENAI_CGO_LDFLAGS); the
# packaged modeld build appends a $ORIGIN-relative rpath into its own lib bundle.
# -rpath-link is link-time only (NOT recorded in the binary): it lets ld resolve
# the transitive deps of the .so we link (e.g. libtbb, needed by libopenvino_genai)
# without baking the venv path — the runtime rpath does that separately.
OPENVINO_GENAI_LINK_FLAGS = -L$(OPENVINO_PKG)/libs -L$(OPENVINO_GENAI_PKG) -L$(OPENVINO_TOKENIZERS_LIB) -l:libopenvino_genai.so.2620 -l:libopenvino.so.2620 -lstdc++ $(OPENVINO_RPATH_DTAGS) -Wl,-rpath-link,$(OPENVINO_PKG)/libs -Wl,-rpath-link,$(OPENVINO_GENAI_PKG) -Wl,-rpath-link,$(OPENVINO_TOKENIZERS_LIB)
OPENVINO_GENAI_CGO_LDFLAGS = $(OPENVINO_GENAI_LINK_FLAGS) -Wl,-rpath,$(OPENVINO_PKG)/libs -Wl,-rpath,$(OPENVINO_GENAI_PKG) -Wl,-rpath,$(OPENVINO_TOKENIZERS_LIB)
