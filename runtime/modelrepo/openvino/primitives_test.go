package openvino

import (
	"errors"
	"testing"

	"github.com/contenox/runtime/runtime/transport"
)

func TestUnit_OpenVINOFatalSessionErrorIncludesTransportFatal(t *testing.T) {
	if !fatalSessionError(transport.ErrSessionFatal) {
		t.Fatal("fatalSessionError should catch transport.ErrSessionFatal")
	}
	if fatalSessionError(errors.New("ordinary error")) {
		t.Fatal("fatalSessionError should not catch ordinary errors")
	}
}
