package vfs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/google/uuid"
)

// ResourceTypeFiles and ResourceTypeFolders tag grants. Both live in one id
// space, so a decision probes both rather than guessing which one an id is.
const (
	ResourceTypeFiles   = "files"
	ResourceTypeFolders = "folders"
)

// maxGrantListCeiling bounds the "return every matching grant" queries, which
// are decided in memory and are not keyset-paginated.
const maxGrantListCeiling = 10000

// AccessCursor is a keyset position in a grant listing. It carries the id as
// well as the timestamp because two grants can share one, and a timestamp-only
// cursor then drops or repeats rows.
type AccessCursor struct {
	CreatedAt time.Time
	ID        string
}

// ACL is the per-resource access list over the VFS: it stores grants and
// answers the one question the [Callbacks] ask — may this actor read or write
// this resource.
type ACL struct {
	db libdbexec.DBManager
}

// NewACL opens the access list over db, creating its table if absent.
func NewACL(ctx context.Context, db libdbexec.DBManager) (*ACL, error) {
	if db == nil {
		return nil, errors.New("vfs: an access list needs a database")
	}
	if err := InitAccessSchema(ctx, db.WithoutTransaction()); err != nil {
		return nil, err
	}
	return &ACL{db: db}, nil
}

// InitAccessSchema creates the grant table, in the spelling the driver needs.
// The table carries no foreign keys: identities and tenants are the host's, and
// a deployment that keeps them elsewhere must not be refused its grants.
func InitAccessSchema(ctx context.Context, exec libdbexec.Exec) error {
	var ddl string
	if exec.DriverName() == "postgres" {
		ddl = `
CREATE TABLE IF NOT EXISTS vfs_access_lists (
    id            VARCHAR(255) PRIMARY KEY,
    tenant_id     VARCHAR(255) NOT NULL,
    identity      VARCHAR(255) NOT NULL,
    resource      TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    permission    INT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, identity, resource_type, resource, permission)
);
CREATE INDEX IF NOT EXISTS vfs_access_lists_identity_idx ON vfs_access_lists (tenant_id, identity);
CREATE INDEX IF NOT EXISTS vfs_access_lists_resource_idx ON vfs_access_lists (tenant_id, resource_type, resource);
CREATE INDEX IF NOT EXISTS vfs_access_lists_created_at_idx ON vfs_access_lists (tenant_id, created_at);
`
	} else {
		ddl = `
CREATE TABLE IF NOT EXISTS vfs_access_lists (
    id            VARCHAR(255) PRIMARY KEY,
    tenant_id     VARCHAR(255) NOT NULL,
    identity      VARCHAR(255) NOT NULL,
    resource      TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    permission    INTEGER NOT NULL,
    created_at    TIMESTAMP NOT NULL,
    updated_at    TIMESTAMP NOT NULL,
    UNIQUE (tenant_id, identity, resource_type, resource, permission)
);
CREATE INDEX IF NOT EXISTS vfs_access_lists_identity_idx ON vfs_access_lists (tenant_id, identity);
CREATE INDEX IF NOT EXISTS vfs_access_lists_resource_idx ON vfs_access_lists (tenant_id, resource_type, resource);
CREATE INDEX IF NOT EXISTS vfs_access_lists_created_at_idx ON vfs_access_lists (tenant_id, created_at);
`
	}
	_, err := exec.ExecContext(ctx, ddl)
	if err != nil {
		return fmt.Errorf("vfs: init access schema: %w", err)
	}
	return nil
}

// Grant writes one grant. ID and timestamps are assigned here; a grant that
// already exists is refused by the unique key rather than silently duplicated.
func (a *ACL) Grant(ctx context.Context, entry AccessEntry) (*AccessEntry, error) {
	if entry.TenantID == "" {
		return nil, errors.New("vfs: a grant requires a tenant")
	}
	if entry.Identity == "" {
		return nil, errors.New("vfs: a grant requires an identity")
	}
	if entry.Resource == "" || entry.ResourceType == "" {
		return nil, errors.New("vfs: a grant requires a resource and a resource type")
	}
	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := a.db.WithoutTransaction().ExecContext(ctx, `
		INSERT INTO vfs_access_lists
			(id, tenant_id, identity, resource, resource_type, permission, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
	`, id, entry.TenantID, entry.Identity, entry.Resource, entry.ResourceType, int(entry.Permission), now); err != nil {
		return nil, fmt.Errorf("vfs: create grant: %w", err)
	}
	entry.ID = id
	entry.CreatedAt = now
	entry.UpdatedAt = now
	return &entry, nil
}

// GetGrant returns one grant by id, scoped to its tenant so a cross-tenant id
// reads as absent.
func (a *ACL) GetGrant(ctx context.Context, tenantID, id string) (*AccessEntry, error) {
	row := a.db.WithoutTransaction().QueryRowContext(ctx, `
		SELECT id, tenant_id, identity, resource, resource_type, permission, created_at, updated_at
		FROM vfs_access_lists
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)
	return scanAccessEntry(row)
}

// UpdateGrant moves a grant's target and permission. Identity and tenant are
// immutable; changing either is a revoke and a fresh grant.
func (a *ACL) UpdateGrant(ctx context.Context, entry AccessEntry) (*AccessEntry, error) {
	if entry.ID == "" || entry.TenantID == "" {
		return nil, errors.New("vfs: updating a grant requires its id and tenant")
	}
	now := time.Now().UTC()
	res, err := a.db.WithoutTransaction().ExecContext(ctx, `
		UPDATE vfs_access_lists
		SET resource = $1, resource_type = $2, permission = $3, updated_at = $4
		WHERE id = $5 AND tenant_id = $6
	`, entry.Resource, entry.ResourceType, int(entry.Permission), now, entry.ID, entry.TenantID)
	if err != nil {
		return nil, fmt.Errorf("vfs: update grant: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, libdbexec.ErrNotFound
	}
	return a.GetGrant(ctx, entry.TenantID, entry.ID)
}

// Revoke removes one grant.
func (a *ACL) Revoke(ctx context.Context, tenantID, id string) error {
	res, err := a.db.WithoutTransaction().ExecContext(ctx,
		`DELETE FROM vfs_access_lists WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("vfs: revoke grant: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return libdbexec.ErrNotFound
	}
	return nil
}

// RevokeByIdentity removes every grant an identity holds in a tenant.
func (a *ACL) RevokeByIdentity(ctx context.Context, tenantID, identity string) error {
	if _, err := a.db.WithoutTransaction().ExecContext(ctx,
		`DELETE FROM vfs_access_lists WHERE tenant_id = $1 AND identity = $2`, tenantID, identity); err != nil {
		return fmt.Errorf("vfs: revoke grants by identity: %w", err)
	}
	return nil
}

// RevokeByResource removes every grant pointing at a resource. A deleted file's
// grants would otherwise outlive it.
func (a *ACL) RevokeByResource(ctx context.Context, tenantID, resourceType, resource string) error {
	if _, err := a.db.WithoutTransaction().ExecContext(ctx, `
		DELETE FROM vfs_access_lists
		WHERE tenant_id = $1 AND resource_type = $2 AND resource = $3
	`, tenantID, resourceType, resource); err != nil {
		return fmt.Errorf("vfs: revoke grants by resource: %w", err)
	}
	return nil
}

// Grants returns a tenant's grants newest first, keyset-paginated. A non-nil
// cursor means another page follows.
func (a *ACL) Grants(ctx context.Context, tenantID string, after *AccessCursor, limit int) ([]*AccessEntry, *AccessCursor, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	exec := a.db.WithoutTransaction()
	var rows *sql.Rows
	var err error
	if after != nil {
		rows, err = exec.QueryContext(ctx, `
			SELECT id, tenant_id, identity, resource, resource_type, permission, created_at, updated_at
			FROM vfs_access_lists
			WHERE tenant_id = $1 AND (created_at, id) < ($2, $3)
			ORDER BY created_at DESC, id DESC
			LIMIT $4
		`, tenantID, after.CreatedAt, after.ID, limit)
	} else {
		rows, err = exec.QueryContext(ctx, `
			SELECT id, tenant_id, identity, resource, resource_type, permission, created_at, updated_at
			FROM vfs_access_lists
			WHERE tenant_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2
		`, tenantID, limit)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("vfs: list grants: %w", err)
	}
	defer rows.Close()
	entries, err := scanAccessEntries(rows)
	if err != nil {
		return nil, nil, err
	}
	var next *AccessCursor
	if len(entries) == limit {
		last := entries[len(entries)-1]
		next = &AccessCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return entries, next, nil
}

// GrantsByIdentity returns every grant an identity holds in a tenant.
func (a *ACL) GrantsByIdentity(ctx context.Context, tenantID, identity string) ([]*AccessEntry, error) {
	rows, err := a.db.WithoutTransaction().QueryContext(ctx, `
		SELECT id, tenant_id, identity, resource, resource_type, permission, created_at, updated_at
		FROM vfs_access_lists
		WHERE tenant_id = $1 AND identity = $2
		ORDER BY created_at DESC
		LIMIT `+fmt.Sprint(maxGrantListCeiling), tenantID, identity)
	if err != nil {
		return nil, fmt.Errorf("vfs: list grants by identity: %w", err)
	}
	defer rows.Close()
	return scanAccessEntries(rows)
}

// matchingGrants returns the grants that could authorize an identity on a
// resource: an exact match, or a wildcard on either half, including the
// tenant-wide "*"/"*".
func (a *ACL) matchingGrants(ctx context.Context, tenantID, identity, resourceType, resource string) ([]*AccessEntry, error) {
	rows, err := a.db.WithoutTransaction().QueryContext(ctx, `
		SELECT id, tenant_id, identity, resource, resource_type, permission, created_at, updated_at
		FROM vfs_access_lists
		WHERE tenant_id = $1
		  AND identity = $2
		  AND (resource_type = $3 OR resource_type = '*')
		  AND (resource = $4 OR resource = '*')
		ORDER BY created_at DESC
		LIMIT `+fmt.Sprint(maxGrantListCeiling), tenantID, identity, resourceType, resource)
	if err != nil {
		return nil, fmt.Errorf("vfs: matching grants: %w", err)
	}
	defer rows.Close()
	return scanAccessEntries(rows)
}

// Allow decides whether actor may perform required on one resource. An admin
// passes without a lookup; a caller acting in another tenant, or with no
// identity, is refused before any query.
func (a *ACL) Allow(ctx context.Context, actor Actor, tenantID, resourceType, resource string, required Permission) error {
	if actor.Admin {
		return nil
	}
	if actor.TenantID == "" || actor.TenantID != tenantID || actor.UserID == "" {
		return ErrAccessDenied
	}
	entries, err := a.matchingGrants(ctx, tenantID, actor.UserID, resourceType, resource)
	if err != nil {
		return err
	}
	if AccessList(entries).Allows(resourceType, resource, required) {
		return nil
	}
	return ErrAccessDenied
}

// allowsEitherType probes files and folders, which share one id space, so a
// decision made from an id alone reaches the grant whichever type it is tagged
// with.
func (a *ACL) allowsEitherType(ctx context.Context, actor Actor, tenantID, resourceID string, required Permission) error {
	if err := a.Allow(ctx, actor, tenantID, ResourceTypeFiles, resourceID, required); err == nil {
		return nil
	} else if !errors.Is(err, ErrAccessDenied) {
		return err
	}
	if err := a.Allow(ctx, actor, tenantID, ResourceTypeFolders, resourceID, required); err == nil {
		return nil
	} else if !errors.Is(err, ErrAccessDenied) {
		return err
	}
	return ErrAccessDenied
}

// deniedAsMissing is what the callbacks return when the actor holds no grant.
// It is libdbexec.ErrNotFound so a surface maps it to "not found": a caller who
// may not see a resource should not be able to probe whether its id exists.
var deniedAsMissing = libdbexec.ErrNotFound

// CallbackOption configures [ACL.Callbacks].
type CallbackOption func(*callbackConfig)

type callbackConfig struct {
	tracker libtracker.ActivityTracker
	onEvent func(ctx context.Context, tenantID, eventType string, file *File) error
}

// EventFileCreated, EventFileUpdated and EventFileDeleted are emitted for every
// mutation that goes through the VFS, whatever wrote it — a session, an upload
// or an importer. The VFS boundary is the one place data-in becomes an event.
const (
	EventFileCreated = "file.created"
	EventFileUpdated = "file.updated"
	EventFileDeleted = "file.deleted"
)

// WithFileEvents emits an event per mutation through emit. Folders are
// structural and emit nothing.
func WithFileEvents(emit func(ctx context.Context, tenantID, eventType string, file *File) error) CallbackOption {
	return func(c *callbackConfig) { c.onEvent = emit }
}

// WithCallbackTracker reports post-commit hook failures, which cannot fail the
// operation that already committed.
func WithCallbackTracker(tracker libtracker.ActivityTracker) CallbackOption {
	return func(c *callbackConfig) { c.tracker = tracker }
}

// Callbacks returns the VFS policy hooks that enforce this access list:
// BeforeRead requires view, BeforeWrite requires edit, a create installs the
// creator's ownership grant, and a delete clears the grants pointing at what
// went away.
//
// The actor comes from the context ([WithActor]), because the hooks are called
// with the operation's context and nothing else. A context carrying no actor
// can neither read nor write: no actor means no grants, and a create by nobody
// is a create the callbacks refuse to attribute.
func (a *ACL) Callbacks(opts ...CallbackOption) Callbacks {
	cfg := callbackConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.tracker == nil {
		cfg.tracker = libtracker.NoopTracker{}
	}

	emit := func(ctx context.Context, tenantID, eventType string, file *File) error {
		if cfg.onEvent == nil || file == nil || file.IsDirectory {
			return nil
		}
		return cfg.onEvent(ctx, tenantID, eventType, file)
	}

	// OnError is the sink for post-commit hook failures: the mutation has
	// committed, so the only honest answer is to report it.
	onError := func(ctx context.Context, tenantID, op, resourceID string, err error) {
		reportErr, _, end := cfg.tracker.Start(ctx, op, "vfs_access",
			"tenantID", tenantID, "resourceID", resourceID)
		defer end()
		reportErr(err)
	}

	return Callbacks{
		BeforeRead: func(ctx context.Context, tenantID, resourceID string) error {
			if resourceID == "" {
				return nil
			}
			actor, ok := ActorFromContext(ctx)
			if !ok {
				return deniedAsMissing
			}
			if err := a.allowsEitherType(ctx, actor, tenantID, resourceID, PermissionView); err != nil {
				return deniedAsMissing
			}
			return nil
		},
		BeforeWrite: func(ctx context.Context, tenantID, resourceID string) error {
			// An empty id is a create: there is nothing to hold a grant yet, and
			// the creator's grant is installed by OnCreate.
			if resourceID == "" {
				return nil
			}
			actor, ok := ActorFromContext(ctx)
			if !ok {
				return deniedAsMissing
			}
			if err := a.allowsEitherType(ctx, actor, tenantID, resourceID, PermissionEdit); err != nil {
				return deniedAsMissing
			}
			return nil
		},
		OnCreate: func(ctx context.Context, tenantID string, file *File) error {
			return errors.Join(a.installOwnership(ctx, tenantID, file), emit(ctx, tenantID, EventFileCreated, file))
		},
		OnUpdate: func(ctx context.Context, tenantID string, file *File) error {
			return emit(ctx, tenantID, EventFileUpdated, file)
		},
		OnDelete: func(ctx context.Context, tenantID, resourceID string) error {
			filesErr := a.RevokeByResource(ctx, tenantID, ResourceTypeFiles, resourceID)
			foldersErr := a.RevokeByResource(ctx, tenantID, ResourceTypeFolders, resourceID)
			// The id alone does not say which type went away, so the deletion
			// event is emitted for both readers to ignore if they never indexed it.
			eventErr := emit(ctx, tenantID, EventFileDeleted, &File{ID: resourceID})
			return errors.Join(filesErr, foldersErr, eventErr)
		},
		OnError: onError,
	}
}

// installOwnership gives the creating identity manage on what it made: whoever
// creates a resource can share it without an operator's help. A create with no
// actor — an importer, a migration — installs nothing; those resources are the
// tenant's and reachable by an admin.
func (a *ACL) installOwnership(ctx context.Context, tenantID string, file *File) error {
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.UserID == "" || file == nil {
		return nil
	}
	resourceType := ResourceTypeFiles
	if file.IsDirectory {
		resourceType = ResourceTypeFolders
	}
	if _, err := a.Grant(ctx, AccessEntry{
		TenantID:     tenantID,
		Identity:     actor.UserID,
		ResourceType: resourceType,
		Resource:     file.ID,
		Permission:   PermissionManage,
	}); err != nil {
		return fmt.Errorf("vfs: install ownership grant for %s/%s: %w", resourceType, file.ID, err)
	}
	return nil
}

func scanAccessEntry(row libdbexec.QueryRower) (*AccessEntry, error) {
	var e AccessEntry
	var perm int
	if err := row.Scan(&e.ID, &e.TenantID, &e.Identity, &e.Resource, &e.ResourceType, &perm, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	e.Permission = Permission(perm)
	return &e, nil
}

func scanAccessEntries(rows *sql.Rows) ([]*AccessEntry, error) {
	var out []*AccessEntry
	for rows.Next() {
		var e AccessEntry
		var perm int
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Identity, &e.Resource, &e.ResourceType, &perm, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Permission = Permission(perm)
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
