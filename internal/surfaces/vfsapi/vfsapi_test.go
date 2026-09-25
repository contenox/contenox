package vfsapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/services/vfs"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/vfsapi"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// actorHeader names the identity a test request acts as. A host's real
// authenticator reads a session; the surface only needs the actor it returns.
const actorHeader = "X-Test-Actor"

type headerActor struct{}

func (headerActor) Actor(r *http.Request) (vfs.Actor, error) {
	raw := r.Header.Get(actorHeader)
	if raw == "" {
		return vfs.Actor{}, vfsapi.ErrNoActor
	}
	var actor vfs.Actor
	if err := json.Unmarshal([]byte(raw), &actor); err != nil {
		return vfs.Actor{}, fmt.Errorf("bad actor header: %w", err)
	}
	return actor, nil
}

type surface struct {
	base   string
	client *http.Client
}

func newSurface(t *testing.T) (*surface, *vfs.ACL) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "vfsapi.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	acl, err := vfs.NewACL(ctx, db)
	require.NoError(t, err)
	svc, err := vfs.New(ctx, db, acl.Callbacks())
	require.NoError(t, err)

	api, err := vfsapi.New(vfsapi.Config{VFS: svc, ACL: acl, Actor: headerActor{}})
	require.NoError(t, err)

	mux := http.NewServeMux()
	sub := http.NewServeMux()
	api.AddRoutes(sub)
	// Mounted the way a host does it: the surface's own paths, under a prefix.
	mux.Handle("/v1/vfs/", http.StripPrefix("/v1/vfs", sub))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return &surface{base: server.URL + "/v1/vfs", client: server.Client()}, acl
}

// as issues a request on behalf of one actor, named in the header the test
// authenticator reads.
func (s *surface) as(t *testing.T, actor vfs.Actor, method, path string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, s.base+path, body)
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	raw, err := json.Marshal(actor)
	require.NoError(t, err)
	req.Header.Set(actorHeader, string(raw))
	resp, err := s.client.Do(req)
	require.NoError(t, err)
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var out T
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func uploadBody(t *testing.T, name, content string, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", name)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, mw.WriteField("name", name))
	for k, v := range fields {
		require.NoError(t, mw.WriteField(k, v))
	}
	require.NoError(t, mw.Close())
	return &buf, mw.FormDataContentType()
}

func alice() vfs.Actor { return vfs.Actor{TenantID: "acme", UserID: "alice"} }

// The whole surface in one pass: an owner uploads, reads, lists, renames and
// downloads; a member of the same tenant is refused until they are granted
// access, and read-only until they are granted more.
func TestSystem_VFSAPI_FilesRoundTripUnderTheAccessList(t *testing.T) {
	s, _ := newSurface(t)

	body, ctype := uploadBody(t, "report.txt", "quarterly numbers", nil)
	resp := s.as(t, alice(), http.MethodPost, "/files", body, ctype)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	created := decode[map[string]any](t, resp)
	id, _ := created["id"].(string)
	require.NotEmpty(t, id)
	assert.Equal(t, "report.txt", created["name"])
	assert.Equal(t, "report.txt", created["path"])
	assert.Equal(t, "text/plain; charset=utf-8", created["contentType"], "the type is detected, not taken from the client")

	resp = s.as(t, alice(), http.MethodGet, "/files", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	listed := decode[[]map[string]any](t, resp)
	require.Len(t, listed, 1)
	assert.Equal(t, id, listed[0]["id"])

	// A member sees nothing: no grant, and the surface answers "not found"
	// rather than confirming the id exists.
	bob := vfs.Actor{TenantID: "acme", UserID: "bob"}
	resp = s.as(t, bob, http.MethodGet, "/files/"+id, nil, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	resp = s.as(t, bob, http.MethodGet, "/files/"+id+"/download", nil, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// The owner shares it for reading.
	grant := map[string]string{"identity": "bob", "resource": id, "resourceType": "files", "permission": "view"}
	raw, _ := json.Marshal(grant)
	resp = s.as(t, alice(), http.MethodPost, "/access-control", bytes.NewReader(raw), "application/json")
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	entry := decode[map[string]any](t, resp)
	grantID, _ := entry["id"].(string)
	require.NotEmpty(t, grantID)

	resp = s.as(t, bob, http.MethodGet, "/files/"+id+"/download", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	downloaded, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, "quarterly numbers", string(downloaded))
	assert.Equal(t, "attachment; filename=\"report.txt\"", resp.Header.Get("Content-Disposition"))
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))

	// Reading is not writing, and writing is not destroying.
	body, ctype = uploadBody(t, "report.txt", "revised", nil)
	resp = s.as(t, bob, http.MethodPut, "/files/"+id, body, ctype)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "view does not reach a write")
	resp.Body.Close()

	resp = s.as(t, bob, http.MethodDelete, "/files/"+id, nil, "")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "deleting takes manage, which bob does not hold")
	resp.Body.Close()

	// The owner renames and moves on, and the listing follows the cursor.
	resp = s.as(t, alice(), http.MethodPut, "/files/"+id+"/name", bytes.NewReader([]byte(`{"name":"q3.txt"}`)), "application/json")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	renamed := decode[map[string]any](t, resp)
	assert.Equal(t, "q3.txt", renamed["name"])
	assert.Equal(t, "q3.txt", renamed["path"])

	resp = s.as(t, alice(), http.MethodDelete, "/files/"+id, nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestSystem_VFSAPI_FoldersAndPaging(t *testing.T) {
	s, _ := newSurface(t)

	raw, _ := json.Marshal(map[string]string{"name": "notes"})
	resp := s.as(t, alice(), http.MethodPost, "/folders", bytes.NewReader(raw), "application/json")
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	folder := decode[map[string]any](t, resp)
	folderID, _ := folder["id"].(string)
	require.NotEmpty(t, folderID)

	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		body, ctype := uploadBody(t, name, "content of "+name, map[string]string{"parentid": folderID})
		resp := s.as(t, alice(), http.MethodPost, "/files", body, ctype)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	resp = s.as(t, alice(), http.MethodGet, "/files?path=notes&limit=2", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	first := decode[[]map[string]any](t, resp)
	require.Len(t, first, 2)
	next := resp.Header.Get("X-Next-Cursor")
	require.NotEmpty(t, next, "a full page hands back the cursor for the next one")

	resp = s.as(t, alice(), http.MethodGet, "/files?path=notes&limit=2&cursor="+next, nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	second := decode[[]map[string]any](t, resp)
	require.Len(t, second, 1)
	assert.NotEqual(t, first[0]["id"], second[0]["id"], "the cursor continues rather than repeating")

	// A folder still holding files refuses to vanish, and an empty one goes.
	resp = s.as(t, alice(), http.MethodDelete, "/folders/"+folderID, nil, "")
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	for _, f := range append(first, second...) {
		resp = s.as(t, alice(), http.MethodDelete, "/files/"+f["id"].(string), nil, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	}
	resp = s.as(t, alice(), http.MethodDelete, "/folders/"+folderID, nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

// An operator acts across tenants and names the one it works in; nobody else
// can name a tenant at all, which is what keeps a session inside its own.
func TestSystem_VFSAPI_OperatorNamesTheTenant(t *testing.T) {
	s, _ := newSurface(t)

	body, ctype := uploadBody(t, "acme.txt", "acme data", nil)
	resp := s.as(t, alice(), http.MethodPost, "/files", body, ctype)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	operator := vfs.Actor{UserID: "operator", Admin: true}
	resp = s.as(t, operator, http.MethodGet, "/files?tenant_id=acme", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	all := decode[[]map[string]any](t, resp)
	require.Len(t, all, 1)
	assert.Equal(t, "acme.txt", all[0]["name"])

	resp = s.as(t, operator, http.MethodGet, "/files", nil, "")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "an operator with no tenant named has nothing to act in")
	resp.Body.Close()

	// A non-operator naming another tenant keeps acting in its own.
	resp = s.as(t, alice(), http.MethodGet, "/files?tenant_id=other", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	own := decode[[]map[string]any](t, resp)
	require.Len(t, own, 1, "the tenant_id parameter is ignored for a session actor")
	resp.Body.Close()
}

func TestSystem_VFSAPI_AnonymousIsRefused(t *testing.T) {
	s, _ := newSurface(t)

	req, err := http.NewRequest(http.MethodGet, s.base+"/files", nil)
	require.NoError(t, err)
	resp, err := s.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestSystem_VFSAPI_AccessControlSurface(t *testing.T) {
	s, _ := newSurface(t)

	resp := s.as(t, alice(), http.MethodGet, "/permissions", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	perms := decode[map[string][]string](t, resp)
	assert.Equal(t, []string{"none", "view", "edit", "manage"}, perms["permissions"])

	body, ctype := uploadBody(t, "shared.txt", "shared", nil)
	resp = s.as(t, alice(), http.MethodPost, "/files", body, ctype)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	file := decode[map[string]any](t, resp)
	id := file["id"].(string)

	// Bob holds no grant on the file, so he cannot share it either.
	raw, _ := json.Marshal(map[string]string{"identity": "bob", "resource": id, "resourceType": "files", "permission": "view"})
	resp = s.as(t, bobActor(), http.MethodPost, "/access-control", bytes.NewReader(raw), "application/json")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()

	// Alice owns it, so she can.
	resp = s.as(t, alice(), http.MethodPost, "/access-control", bytes.NewReader(raw), "application/json")
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	entry := decode[map[string]any](t, resp)
	grantID := entry["id"].(string)
	assert.Equal(t, "view", entry["permission"])
	assert.Equal(t, "files", entry["resourceType"])

	// A malformed permission is refused at the edge, not stored.
	raw, _ = json.Marshal(map[string]string{"identity": "bob", "resource": id, "resourceType": "files", "permission": "owner"})
	resp = s.as(t, alice(), http.MethodPost, "/access-control", bytes.NewReader(raw), "application/json")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Bob reads his own grants and no one else's.
	resp = s.as(t, bobActor(), http.MethodGet, "/access-control", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	own := decode[map[string][]map[string]any](t, resp)
	require.Len(t, own["entries"], 1)
	assert.Equal(t, "bob", own["entries"][0]["identity"])

	// An operator sees every grant in the tenant.
	operator := vfs.Actor{UserID: "operator", Admin: true}
	resp = s.as(t, operator, http.MethodGet, "/access-control?tenant_id=acme", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	every := decode[map[string][]map[string]any](t, resp)
	assert.Len(t, every["entries"], 2, "the owner's own grant and the one she made")

	// Alice revokes what she granted.
	resp = s.as(t, alice(), http.MethodDelete, "/access-control/"+grantID, nil, "")
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	resp = s.as(t, bobActor(), http.MethodGet, "/files/"+id, nil, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "the revoked member is a stranger again")
	resp.Body.Close()
}

func bobActor() vfs.Actor { return vfs.Actor{TenantID: "acme", UserID: "bob"} }
