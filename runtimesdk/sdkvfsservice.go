package runtimesdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/contenox/contenox/apiframework"
	"github.com/contenox/contenox/vfsservice"
)

// HTTPVFSService implements vfsservice.Service over HTTP.
type HTTPVFSService struct {
	client  *http.Client
	baseURL string
	token   string
}

// NewHTTPVFSService creates a new HTTP client that implements vfsservice.Service.
func NewHTTPVFSService(baseURL, token string, client *http.Client) vfsservice.Service {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPVFSService{
		client:  client,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
	}
}

var _ vfsservice.Service = (*HTTPVFSService)(nil)

func (s *HTTPVFSService) setAuth(req *http.Request) {
	if s.token != "" {
		req.Header.Set("X-API-Key", s.token)
	}
}

func (s *HTTPVFSService) doJSON(req *http.Request, expectStatus int, out any) error {
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectStatus {
		return apiframework.HandleAPIError(resp)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// fileResp mirrors vfsapi.FileResponse for JSON decoding.
type fileResp struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

func (r fileResp) toFile() *vfsservice.File {
	return &vfsservice.File{
		ID:          r.ID,
		Path:        r.Path,
		Name:        r.Name,
		ContentType: r.ContentType,
		Size:        r.Size,
	}
}

// folderResp mirrors vfsapi.FolderResponse for JSON decoding.
type folderResp struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
}

func (r folderResp) toFolder() *vfsservice.Folder {
	return &vfsservice.Folder{
		ID:       r.ID,
		Path:     r.Path,
		Name:     r.Name,
		ParentID: r.ParentID,
	}
}

// uploadFile sends a multipart/form-data request with the file content.
func (s *HTTPVFSService) uploadFile(ctx context.Context, method, url string, file *vfsservice.File, expectStatus int) (*vfsservice.File, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	if file.Name != "" {
		_ = w.WriteField("name", file.Name)
	}
	if file.ParentID != "" {
		_ = w.WriteField("parentid", file.ParentID)
	}
	part, err := w.CreateFormFile("file", file.Name)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(file.Data)); err != nil {
		return nil, fmt.Errorf("write file data: %w", err)
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, method, url, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	var fr fileResp
	if err := s.doJSON(req, expectStatus, &fr); err != nil {
		return nil, err
	}
	return fr.toFile(), nil
}

func (s *HTTPVFSService) CreateFile(ctx context.Context, file *vfsservice.File) (*vfsservice.File, error) {
	return s.uploadFile(ctx, http.MethodPost, s.baseURL+"/files", file, http.StatusCreated)
}

func (s *HTTPVFSService) GetFileByID(ctx context.Context, id string) (*vfsservice.File, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/files/%s", s.baseURL, id), nil)
	if err != nil {
		return nil, err
	}
	var fr fileResp
	if err := s.doJSON(req, http.StatusOK, &fr); err != nil {
		return nil, err
	}
	return fr.toFile(), nil
}

func (s *HTTPVFSService) GetFolderByID(ctx context.Context, id string) (*vfsservice.Folder, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/folders/%s", s.baseURL, id), nil)
	if err != nil {
		return nil, err
	}
	var fr folderResp
	if err := s.doJSON(req, http.StatusOK, &fr); err != nil {
		return nil, err
	}
	return fr.toFolder(), nil
}

func (s *HTTPVFSService) GetFilesByPath(ctx context.Context, path string) ([]vfsservice.File, error) {
	url := s.baseURL + "/files"
	if path != "" {
		url += "?path=" + path
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	var items []fileResp
	if err := s.doJSON(req, http.StatusOK, &items); err != nil {
		return nil, err
	}
	files := make([]vfsservice.File, len(items))
	for i, item := range items {
		files[i] = *item.toFile()
	}
	return files, nil
}

func (s *HTTPVFSService) UpdateFile(ctx context.Context, file *vfsservice.File) (*vfsservice.File, error) {
	return s.uploadFile(ctx, http.MethodPut, fmt.Sprintf("%s/files/%s", s.baseURL, file.ID), file, http.StatusOK)
}

func (s *HTTPVFSService) DeleteFile(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/files/%s", s.baseURL, id), nil)
	if err != nil {
		return err
	}
	return s.doJSON(req, http.StatusOK, nil)
}

func (s *HTTPVFSService) CreateFolder(ctx context.Context, parentID, name string) (*vfsservice.Folder, error) {
	body, _ := json.Marshal(map[string]string{"name": name, "parentId": parentID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/folders", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var fr folderResp
	if err := s.doJSON(req, http.StatusCreated, &fr); err != nil {
		return nil, err
	}
	return fr.toFolder(), nil
}

func (s *HTTPVFSService) RenameFile(ctx context.Context, fileID, newName string) (*vfsservice.File, error) {
	body, _ := json.Marshal(map[string]string{"name": newName})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/files/%s/name", s.baseURL, fileID), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var fr fileResp
	if err := s.doJSON(req, http.StatusOK, &fr); err != nil {
		return nil, err
	}
	return fr.toFile(), nil
}

func (s *HTTPVFSService) RenameFolder(ctx context.Context, folderID, newName string) (*vfsservice.Folder, error) {
	body, _ := json.Marshal(map[string]string{"name": newName})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/folders/%s/name", s.baseURL, folderID), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var fr folderResp
	if err := s.doJSON(req, http.StatusOK, &fr); err != nil {
		return nil, err
	}
	return fr.toFolder(), nil
}

func (s *HTTPVFSService) DeleteFolder(ctx context.Context, folderID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/folders/%s", s.baseURL, folderID), nil)
	if err != nil {
		return err
	}
	return s.doJSON(req, http.StatusOK, nil)
}

func (s *HTTPVFSService) MoveFile(ctx context.Context, fileID, newParentID string) (*vfsservice.File, error) {
	body, _ := json.Marshal(map[string]string{"newParentId": newParentID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/files/%s/move", s.baseURL, fileID), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var fr fileResp
	if err := s.doJSON(req, http.StatusOK, &fr); err != nil {
		return nil, err
	}
	return fr.toFile(), nil
}

func (s *HTTPVFSService) MoveFolder(ctx context.Context, folderID, newParentID string) (*vfsservice.Folder, error) {
	body, _ := json.Marshal(map[string]string{"newParentId": newParentID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/folders/%s/move", s.baseURL, folderID), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var fr folderResp
	if err := s.doJSON(req, http.StatusOK, &fr); err != nil {
		return nil, err
	}
	return fr.toFolder(), nil
}
