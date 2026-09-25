package vfsapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/contenox/contenox/internal/services/vfs"
)

const (
	formFieldFile     = "file"
	formFieldName     = "name"
	formFieldParent   = "parentid"
	formFieldMetadata = "metadata"

	multipartFormMemory = 8 << 20

	// maxRequestSize bounds the whole multipart body. The store's own
	// MaxUploadSize bounds one file; the request is a little larger because of
	// multipart framing and the form fields beside it.
	maxRequestSize = vfs.MaxUploadSize + 10*1024
)

type uploadFields struct {
	data        []byte
	size        int64
	name        string
	parentID    string
	contentType string
	metadata    map[string]string
	// metadataSet distinguishes an absent metadata field from an explicit empty
	// one, which is what lets an update preserve stored metadata.
	metadataSet bool
}

// readUpload parses a multipart body with a required "file" part and optional
// "name", "parentid" and "metadata" fields.
//
// The declared content type is ignored: the type is detected from the bytes, so
// a client cannot have its upload stored as something it is not. The body is
// bounded twice — once for the request, once for the part — because a part
// header can claim a size the payload does not honour.
func readUpload(w http.ResponseWriter, r *http.Request) (*uploadFields, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)

	if err := r.ParseMultipartForm(multipartFormMemory); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, fmt.Errorf("request body too large (limit %d bytes)", maxBytesErr.Limit)
		}
		if errors.Is(err, http.ErrNotMultipart) {
			return nil, errors.New("invalid request format (not multipart)")
		}
		return nil, fmt.Errorf("failed to parse multipart form: %w", err)
	}

	part, header, err := r.FormFile(formFieldFile)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return nil, fmt.Errorf("missing required field %q", formFieldFile)
		}
		return nil, fmt.Errorf("invalid file upload: %w", err)
	}
	defer part.Close()

	if header.Size == 0 {
		return nil, errors.New("file is empty")
	}
	if header.Size > vfs.MaxUploadSize {
		return nil, fmt.Errorf("file exceeds max upload size %d", int64(vfs.MaxUploadSize))
	}

	data, err := io.ReadAll(io.LimitReader(part, vfs.MaxUploadSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}
	if int64(len(data)) > vfs.MaxUploadSize {
		return nil, fmt.Errorf("file exceeds max upload size %d", int64(vfs.MaxUploadSize))
	}

	name := r.FormValue(formFieldName)
	if name == "" {
		name = header.Filename
	}
	metadata, err := parseMetadataField(r.FormValue(formFieldMetadata))
	if err != nil {
		return nil, err
	}

	return &uploadFields{
		data:        data,
		size:        int64(len(data)),
		name:        name,
		parentID:    r.FormValue(formFieldParent),
		contentType: http.DetectContentType(data),
		metadata:    metadata,
		metadataSet: r.Form.Has(formFieldMetadata),
	}, nil
}

// parseMetadataField decodes the optional metadata field: a JSON object of
// string values stored beside the content, which is where a title or an
// importer's source-version token rides.
func parseMetadataField(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("field %q must be a JSON object of string values: %w", formFieldMetadata, err)
	}
	return m, nil
}
