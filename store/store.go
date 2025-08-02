package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"

	libdb "github.com/contenox/dbexec"
	"github.com/stretchr/testify/require"
)

type Status struct {
	Status    string `json:"status"`
	Digest    string `json:"digest,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Completed int64  `json:"completed,omitempty"`
	Model     string `json:"model"`
	BaseURL   string `json:"baseUrl"`
}

type QueueItem struct {
	URL   string `json:"url"`
	Model string `json:"model"`
}

type Backend struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Type    string `json:"type"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Model struct {
	ID        string    `json:"id"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Pool struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PurposeType string `json:"purposeType"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Job struct {
	ID           string    `json:"id"`
	TaskType     string    `json:"taskType"`
	Operation    string    `json:"operation"`
	Subject      string    `json:"subject"`
	EntityID     string    `json:"entityId"`
	EntityType   string    `json:"entityType"`
	Payload      []byte    `json:"payload"`
	ScheduledFor int64     `json:"scheduledFor"`
	ValidUntil   int64     `json:"validUntil"`
	RetryCount   int       `json:"retryCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

type LeasedJob struct {
	Job
	Leaser          string    `json:"leaser"`
	LeaseExpiration time.Time `json:"leaseExpiration"`
}

type Resource struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

const (
	ResourceTypeSystem = "system"
	ResourceTypeFiles  = "files"
	ResourceTypeFile   = "file"
	ResourceTypeBlobs  = "blobs"
	ResourceTypeChunks = "chunks"
)

var ResourceTypes = []string{
	ResourceTypeSystem,
	ResourceTypeFiles,
	ResourceTypeBlobs,
	ResourceTypeChunks,
}

type Blob struct {
	ID        string    `json:"id"`
	Meta      []byte    `json:"meta"`
	Data      []byte    `json:"data"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// KV represents a key-value pair in the database
type KV struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type Store interface {
	CreateBackend(ctx context.Context, backend *Backend) error
	GetBackend(ctx context.Context, id string) (*Backend, error)
	UpdateBackend(ctx context.Context, backend *Backend) error
	DeleteBackend(ctx context.Context, id string) error
	ListBackends(ctx context.Context) ([]*Backend, error)
	GetBackendByName(ctx context.Context, name string) (*Backend, error)

	AppendModel(ctx context.Context, model *Model) error
	GetModel(ctx context.Context, id string) (*Model, error)
	GetModelByName(ctx context.Context, name string) (*Model, error)
	DeleteModel(ctx context.Context, modelName string) error
	ListModels(ctx context.Context) ([]*Model, error)

	CreatePool(ctx context.Context, pool *Pool) error
	GetPool(ctx context.Context, id string) (*Pool, error)
	GetPoolByName(ctx context.Context, name string) (*Pool, error)
	UpdatePool(ctx context.Context, pool *Pool) error
	DeletePool(ctx context.Context, id string) error
	ListPools(ctx context.Context) ([]*Pool, error)
	ListPoolsByPurpose(ctx context.Context, purposeType string) ([]*Pool, error)

	AssignBackendToPool(ctx context.Context, poolID string, backendID string) error
	RemoveBackendFromPool(ctx context.Context, poolID string, backendID string) error
	ListBackendsForPool(ctx context.Context, poolID string) ([]*Backend, error)
	ListPoolsForBackend(ctx context.Context, backendID string) ([]*Pool, error)

	AssignModelToPool(ctx context.Context, poolID string, modelID string) error
	RemoveModelFromPool(ctx context.Context, poolID string, modelID string) error
	ListModelsForPool(ctx context.Context, poolID string) ([]*Model, error)
	ListPoolsForModel(ctx context.Context, modelID string) ([]*Pool, error)

	AppendJob(ctx context.Context, job Job) error
	AppendJobs(ctx context.Context, jobs ...*Job) error
	PopAllJobs(ctx context.Context) ([]*Job, error)
	PopJobsForType(ctx context.Context, taskType string) ([]*Job, error)
	PopNJobsForType(ctx context.Context, taskType string, n int) ([]*Job, error)
	PopJobForType(ctx context.Context, taskType string) (*Job, error)
	GetJobsForType(ctx context.Context, taskType string) ([]*Job, error)
	ListJobs(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*Job, error)
	DeleteJobsByEntity(ctx context.Context, entityID, entityType string) error

	AppendLeasedJob(ctx context.Context, job Job, duration time.Duration, leaser string) error
	GetLeasedJob(ctx context.Context, id string) (*LeasedJob, error)
	DeleteLeasedJob(ctx context.Context, id string) error
	ListLeasedJobs(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*LeasedJob, error)
	DeleteLeasedJobs(ctx context.Context, entityID, entityType string) error

	SetKV(ctx context.Context, key string, value json.RawMessage) error
	UpdateKV(ctx context.Context, key string, value json.RawMessage) error
	GetKV(ctx context.Context, key string, out interface{}) error
	DeleteKV(ctx context.Context, key string) error
	ListKV(ctx context.Context) ([]*KV, error)
	ListKVPrefix(ctx context.Context, prefix string) ([]*KV, error)
}

//go:embed schema.sql
var Schema string

type store struct {
	libdb.Exec
}

func New(exec libdb.Exec) Store {
	if exec == nil {
		panic("SERVER BUG: store.New called with nil exec")
	}
	return &store{exec}
}

func quiet() func() {
	null, _ := os.Open(os.DevNull)
	sout := os.Stdout
	serr := os.Stderr
	os.Stdout = null
	os.Stderr = null
	log.SetOutput(null)
	return func() {
		defer null.Close()
		os.Stdout = sout
		os.Stderr = serr
		log.SetOutput(os.Stderr)
	}
}

// setupStore initializes a test Postgres instance and returns the store.
func SetupStore(t *testing.T) (context.Context, Store) {
	t.Helper()

	// Silence logs
	unquiet := quiet()
	t.Cleanup(unquiet)

	ctx := context.TODO()
	connStr, _, cleanup, err := libdb.SetupLocalInstance(ctx, "test", "test", "test")
	require.NoError(t, err)

	dbManager, err := libdb.NewPostgresDBManager(ctx, connStr, Schema)
	require.NoError(t, err)

	// Cleanup DB and container
	t.Cleanup(func() {
		require.NoError(t, dbManager.Close())
		cleanup()
	})

	s := New(dbManager.WithoutTransaction())
	return ctx, s
}
