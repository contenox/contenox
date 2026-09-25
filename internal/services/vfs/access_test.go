package vfs_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/services/vfs"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func accessFixture(t *testing.T) (context.Context, vfs.Service, *vfs.ACL, string) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "acl.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	acl, err := vfs.NewACL(ctx, db)
	require.NoError(t, err)
	svc, err := vfs.New(ctx, db, acl.Callbacks())
	require.NoError(t, err)
	return ctx, svc, acl, "tenant-one"
}

func as(tenantID, userID string) context.Context {
	return vfs.WithActor(context.Background(), vfs.Actor{TenantID: tenantID, UserID: userID})
}

// The access list is only worth its table if the VFS consults it: a create by
// an identity makes that identity the owner, and everyone else is refused the
// resource as though it did not exist.
func TestUnit_ACL_OwnershipIsInstalledByCreating(t *testing.T) {
	ctx, svc, acl, tenant := accessFixture(t)

	alice := as(tenant, "alice")
	file, err := svc.CreateFile(alice, tenant, &vfs.File{Name: "plan.md", Data: []byte("v1")})
	require.NoError(t, err)

	grants, err := acl.GrantsByIdentity(ctx, tenant, "alice")
	require.NoError(t, err)
	require.Len(t, grants, 1, "creating installs exactly one grant")
	assert.Equal(t, vfs.ResourceTypeFiles, grants[0].ResourceType)
	assert.Equal(t, file.ID, grants[0].Resource)
	assert.Equal(t, vfs.PermissionManage, grants[0].Permission, "an owner can share what it made")

	bob := as(tenant, "bob")
	_, err = svc.GetFileByID(bob, tenant, file.ID)
	assert.ErrorIs(t, err, libdb.ErrNotFound,
		"a caller with no grant is told the resource does not exist, not that it is forbidden")
}

func TestUnit_ACL_ViewReadsWithoutWriting(t *testing.T) {
	ctx, svc, acl, tenant := accessFixture(t)

	alice := as(tenant, "alice")
	file, err := svc.CreateFile(alice, tenant, &vfs.File{Name: "shared.txt", Data: []byte("v1")})
	require.NoError(t, err)

	_, err = acl.Grant(ctx, vfs.AccessEntry{
		TenantID: tenant, Identity: "bob", ResourceType: vfs.ResourceTypeFiles,
		Resource: file.ID, Permission: vfs.PermissionView,
	})
	require.NoError(t, err)

	bob := as(tenant, "bob")
	got, err := svc.GetFileByID(bob, tenant, file.ID)
	require.NoError(t, err)
	assert.Equal(t, "v1", string(got.Data))

	_, err = svc.UpdateFile(bob, tenant, &vfs.File{ID: file.ID, Data: []byte("v2")})
	assert.ErrorIs(t, err, libdb.ErrNotFound, "view is not edit")

	_, err = acl.Grant(ctx, vfs.AccessEntry{
		TenantID: tenant, Identity: "bob", ResourceType: vfs.ResourceTypeFiles,
		Resource: file.ID, Permission: vfs.PermissionEdit,
	})
	require.NoError(t, err)
	_, err = svc.UpdateFile(bob, tenant, &vfs.File{ID: file.ID, Data: []byte("v2")})
	assert.NoError(t, err, "edit is enough to write")

	// The callbacks gate a delete as a write, so edit reaches it. "Deleting
	// needs manage" is a rule of the HTTP surface, which checks it before it
	// calls the VFS: manage subsumes edit, and the boundary cannot tell a
	// delete from an update.
	require.NoError(t, svc.DeleteFile(bob, tenant, file.ID))
}

func TestUnit_ACL_PermissionOrderAndWildcards(t *testing.T) {
	ctx, svc, acl, tenant := accessFixture(t)

	alice := as(tenant, "alice")
	first, err := svc.CreateFile(alice, tenant, &vfs.File{Name: "a.txt", Data: []byte("a")})
	require.NoError(t, err)
	second, err := svc.CreateFile(alice, tenant, &vfs.File{Name: "b.txt", Data: []byte("b")})
	require.NoError(t, err)

	_, err = acl.Grant(ctx, vfs.AccessEntry{
		TenantID: tenant, Identity: "carol", ResourceType: vfs.Wildcard,
		Resource: vfs.Wildcard, Permission: vfs.PermissionView,
	})
	require.NoError(t, err)

	carol := as(tenant, "carol")
	for _, id := range []string{first.ID, second.ID} {
		_, err := svc.GetFileByID(carol, tenant, id)
		assert.NoError(t, err, "a tenant-wide grant reaches every resource")
	}

	// manage is a superset of view: one grant answers a weaker requirement.
	require.NoError(t, acl.Allow(ctx, vfs.Actor{TenantID: tenant, UserID: "carol"}, tenant,
		vfs.ResourceTypeFiles, first.ID, vfs.PermissionView))
	assert.NoError(t, acl.Allow(ctx, vfs.Actor{TenantID: tenant, UserID: "alice"}, tenant,
		vfs.ResourceTypeFiles, first.ID, vfs.PermissionManage))
}

func TestUnit_ACL_StrangersAndAdmins(t *testing.T) {
	_, svc, _, tenant := accessFixture(t)

	file, err := svc.CreateFile(as(tenant, "alice"), tenant, &vfs.File{Name: "secret.txt", Data: []byte("s")})
	require.NoError(t, err)

	otherTenant := as("tenant-two", "alice")
	_, err = svc.GetFileByID(otherTenant, tenant, file.ID)
	assert.ErrorIs(t, err, libdb.ErrNotFound, "a grant in one tenant never reaches another")

	_, err = svc.GetFileByID(context.Background(), tenant, file.ID)
	assert.ErrorIs(t, err, libdb.ErrNotFound, "a call with no actor has no grants")

	admin := vfs.WithActor(context.Background(), vfs.Actor{TenantID: tenant, UserID: "operator", Admin: true})
	got, err := svc.GetFileByID(admin, tenant, file.ID)
	require.NoError(t, err)
	assert.Equal(t, "secret.txt", got.Name)

	// An admin may also write and delete without holding a grant.
	_, err = svc.UpdateFile(admin, tenant, &vfs.File{ID: file.ID, Data: []byte("edited")})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteFile(admin, tenant, file.ID))
}

// A deleted resource's grants must not outlive it, or a later id reused by the
// store would inherit someone else's access.
func TestUnit_ACL_DeleteClearsTheGrants(t *testing.T) {
	ctx, svc, acl, tenant := accessFixture(t)

	alice := as(tenant, "alice")
	file, err := svc.CreateFile(alice, tenant, &vfs.File{Name: "temp.txt", Data: []byte("t")})
	require.NoError(t, err)

	grants, _, err := acl.Grants(ctx, tenant, nil, 100)
	require.NoError(t, err)
	require.Len(t, grants, 1)

	require.NoError(t, svc.DeleteFile(alice, tenant, file.ID))

	grants, _, err = acl.Grants(ctx, tenant, nil, 100)
	require.NoError(t, err)
	assert.Empty(t, grants, "both the file's grants and its owner's are gone with it")
}

// Folders share the id space with files, so a grant tagged with either type has
// to answer for an id whose type the caller cannot see.
func TestUnit_ACL_FolderGrantsAnswerForFolderIDs(t *testing.T) {
	ctx, svc, acl, tenant := accessFixture(t)

	alice := as(tenant, "alice")
	folder, err := svc.CreateFolder(alice, tenant, "", "notes")
	require.NoError(t, err)

	grants, err := acl.GrantsByIdentity(ctx, tenant, "alice")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	assert.Equal(t, vfs.ResourceTypeFolders, grants[0].ResourceType)

	bob := as(tenant, "bob")
	_, err = svc.GetFolderByID(bob, tenant, folder.ID)
	assert.ErrorIs(t, err, libdb.ErrNotFound)

	_, err = acl.Grant(ctx, vfs.AccessEntry{
		TenantID: tenant, Identity: "bob", ResourceType: vfs.ResourceTypeFolders,
		Resource: folder.ID, Permission: vfs.PermissionView,
	})
	require.NoError(t, err)

	got, err := svc.GetFolderByID(bob, tenant, folder.ID)
	require.NoError(t, err)
	assert.Equal(t, "notes", got.Name)
}

func TestUnit_ACL_PermissionWireForm(t *testing.T) {
	assert.Equal(t, []string{"none", "view", "edit", "manage"}, vfs.PermissionStrings())
	for want, name := range map[vfs.Permission]string{
		vfs.PermissionNone: "none", vfs.PermissionView: "view",
		vfs.PermissionEdit: "edit", vfs.PermissionManage: "manage",
	} {
		assert.Equal(t, name, want.String())
		parsed, err := vfs.PermissionFromString(name)
		require.NoError(t, err)
		assert.Equal(t, want, parsed)
	}
	_, err := vfs.PermissionFromString("owner")
	assert.Error(t, err)
}
