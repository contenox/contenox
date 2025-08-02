package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/contenox/runtime/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUnit_JobQueue_AppendJobAndPopAll(t *testing.T) {
	ctx, s := store.SetupStore(t)

	job := &store.Job{
		ID:           uuid.New().String(),
		TaskType:     "test-task",
		Payload:      []byte(`{"key": "value"}`),
		Operation:    "create",
		Subject:      "user",
		EntityID:     uuid.New().String(),
		EntityType:   "test",
		ScheduledFor: 1620000000,
		ValidUntil:   1620003600,
		RetryCount:   0,
	}

	// Append the job.
	err := s.AppendJob(ctx, *job)
	require.NoError(t, err)

	// Pop all jobs.
	jobs, err := s.PopAllJobs(ctx)
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	retrieved := jobs[0]
	require.Equal(t, job.TaskType, retrieved.TaskType)
	require.Equal(t, job.Payload, retrieved.Payload)
	require.Equal(t, job.Operation, retrieved.Operation)
	require.Equal(t, job.Subject, retrieved.Subject)
	require.Equal(t, job.EntityID, retrieved.EntityID)
	require.Equal(t, job.EntityType, retrieved.EntityType)
	require.Equal(t, job.ScheduledFor, retrieved.ScheduledFor)
	require.Equal(t, job.ValidUntil, retrieved.ValidUntil)
	require.Equal(t, job.RetryCount, retrieved.RetryCount)
}

func TestUnit_JobQueue_PopAllForType(t *testing.T) {
	ctx, s := store.SetupStore(t)

	job1 := &store.Job{
		ID:           uuid.New().String(),
		TaskType:     "type-A",
		Payload:      []byte(`{"foo": "bar"}`),
		ScheduledFor: 1610000000,
		ValidUntil:   1610003600,
	}
	job2 := &store.Job{
		ID:           uuid.New().String(),
		TaskType:     "type-B",
		Payload:      []byte(`{"hello": "world"}`),
		ScheduledFor: 1620000000,
		ValidUntil:   1620003600,
	}

	require.NoError(t, s.AppendJob(ctx, *job1))
	require.NoError(t, s.AppendJob(ctx, *job2))

	// Pop jobs of type-A.
	jobs, err := s.PopJobsForType(ctx, "type-A")
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	retrieved := jobs[0]
	require.Equal(t, job1.TaskType, retrieved.TaskType)
	require.Equal(t, job1.Payload, retrieved.Payload)
	require.Equal(t, job1.ScheduledFor, retrieved.ScheduledFor)
	require.Equal(t, job1.ValidUntil, retrieved.ValidUntil)

	// Ensure type-B job is still in the queue.
	remainingJobs, err := s.PopAllJobs(ctx)
	require.NoError(t, err)
	require.Len(t, remainingJobs, 1)
	require.Equal(t, job2.TaskType, remainingJobs[0].TaskType)
}

func TestUnit_JobQueue_PopAllEmptyQueue(t *testing.T) {
	ctx, s := store.SetupStore(t)

	jobs, err := s.PopAllJobs(ctx)
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func TestUnit_JobQueue_PopAllForTypeEmpty(t *testing.T) {
	ctx, s := store.SetupStore(t)

	jobs, err := s.PopJobsForType(ctx, "non-existent-type")
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func TestUnit_JobQueue_PopOneForType(t *testing.T) {
	ctx, s := store.SetupStore(t)

	// Prepare valid JSON payloads.
	job1Payload, _ := json.Marshal(map[string]string{"data": "job1"})
	job2Payload, _ := json.Marshal(map[string]string{"data": "job2"})
	job3Payload, _ := json.Marshal(map[string]string{"data": "job3"})

	// Insert three jobs: two of type "task-A", one of type "task-B".
	job1 := store.Job{
		ID:           uuid.New().String(),
		TaskType:     "task-A",
		Payload:      job1Payload,
		ScheduledFor: 1600000000,
		ValidUntil:   1600003600,
	}
	job2 := store.Job{
		ID:           uuid.New().String(),
		TaskType:     "task-A",
		Payload:      job2Payload,
		ScheduledFor: 1600000001,
		ValidUntil:   1600003601,
	}
	job3 := store.Job{
		ID:           uuid.New().String(),
		TaskType:     "task-B",
		Payload:      job3Payload,
		ScheduledFor: 1600000002,
		ValidUntil:   1600003602,
	}

	require.NoError(t, s.AppendJob(ctx, job1))
	time.Sleep(10 * time.Millisecond) // Ensure ordering by created_at.
	require.NoError(t, s.AppendJob(ctx, job2))
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, s.AppendJob(ctx, job3))

	// Pop one job of type "task-A" (oldest should be returned).
	poppedJob, err := s.PopJobForType(ctx, "task-A")
	require.NoError(t, err)
	require.NotNil(t, poppedJob)
	require.Equal(t, "task-A", poppedJob.TaskType)
	require.Equal(t, job1.ID, poppedJob.ID)
	require.Equal(t, job1.ScheduledFor, poppedJob.ScheduledFor)
	require.Equal(t, job1.ValidUntil, poppedJob.ValidUntil)

	// Pop another job of type "task-A".
	poppedJob2, err := s.PopJobForType(ctx, "task-A")
	require.NoError(t, err)
	require.NotNil(t, poppedJob2)
	require.Equal(t, "task-A", poppedJob2.TaskType)
	require.Equal(t, job2.ID, poppedJob2.ID)
	require.Equal(t, job2.ScheduledFor, poppedJob2.ScheduledFor)
	require.Equal(t, job2.ValidUntil, poppedJob2.ValidUntil)

	// Try popping another "task-A" job (should return an error or no rows).
	poppedJob3, err := s.PopJobForType(ctx, "task-A")
	require.Error(t, err)
	require.Nil(t, poppedJob3)

	// Ensure "task-B" job is still available.
	poppedJobB, err := s.PopJobForType(ctx, "task-B")
	require.NoError(t, err)
	require.NotNil(t, poppedJobB)
	require.Equal(t, "task-B", poppedJobB.TaskType)
	require.Equal(t, job3.ID, poppedJobB.ID)
	require.Equal(t, job3.ScheduledFor, poppedJobB.ScheduledFor)
	require.Equal(t, job3.ValidUntil, poppedJobB.ValidUntil)
}

func TestUnit_JobQueue_GetAllForType(t *testing.T) {
	ctx, s := store.SetupStore(t)

	// Prepare valid JSON payloads.
	payloadA1, err := json.Marshal(map[string]string{"job": "A1"})
	require.NoError(t, err)
	payloadA2, err := json.Marshal(map[string]string{"job": "A2"})
	require.NoError(t, err)
	payloadB, err := json.Marshal(map[string]string{"job": "B"})
	require.NoError(t, err)

	// Insert two jobs of type "task-A" and one job of type "task-B".
	jobA1 := store.Job{
		ID:           uuid.New().String(),
		TaskType:     "task-A",
		Payload:      payloadA1,
		ScheduledFor: 1630000000,
		ValidUntil:   1630003600,
	}
	jobA2 := store.Job{
		ID:           uuid.New().String(),
		TaskType:     "task-A",
		Payload:      payloadA2,
		ScheduledFor: 1630000001,
		ValidUntil:   1630003601,
	}
	jobB := store.Job{
		ID:           uuid.New().String(),
		TaskType:     "task-B",
		Payload:      payloadB,
		ScheduledFor: 1630000002,
		ValidUntil:   1630003602,
	}

	require.NoError(t, s.AppendJob(ctx, jobA1))
	time.Sleep(10 * time.Millisecond) // Ensure different created_at timestamps.
	require.NoError(t, s.AppendJob(ctx, jobA2))
	require.NoError(t, s.AppendJob(ctx, jobB))

	// Retrieve all jobs of type "task-A" without deletion.
	jobsA, err := s.GetJobsForType(ctx, "task-A")
	require.NoError(t, err)
	require.Len(t, jobsA, 2)

	// Ensure the jobs are returned in order of creation.
	require.Equal(t, jobA1.ID, jobsA[0].ID)
	require.Equal(t, jobA2.ID, jobsA[1].ID)
	// Check that scheduledFor and validUntil are correct.
	require.Equal(t, jobA1.ScheduledFor, jobsA[0].ScheduledFor)
	require.Equal(t, jobA1.ValidUntil, jobsA[0].ValidUntil)
	require.Equal(t, jobA2.ScheduledFor, jobsA[1].ScheduledFor)
	require.Equal(t, jobA2.ValidUntil, jobsA[1].ValidUntil)

	// Calling GetJobsForType again should return the same jobs.
	jobsAAgain, err := s.GetJobsForType(ctx, "task-A")
	require.NoError(t, err)
	require.Len(t, jobsAAgain, 2)

	// Retrieve jobs for "task-B".
	jobsB, err := s.GetJobsForType(ctx, "task-B")
	require.NoError(t, err)
	require.Len(t, jobsB, 1)
	require.Equal(t, jobB.ID, jobsB[0].ID)
	require.Equal(t, jobB.ScheduledFor, jobsB[0].ScheduledFor)
	require.Equal(t, jobB.ValidUntil, jobsB[0].ValidUntil)
}

func newTestUnit_JobQueue_Job(taskType string) *store.Job {
	return &store.Job{
		ID:        uuid.New().String(),
		TaskType:  taskType,
		Payload:   []byte(`{}`),
		Operation: "test-op",
		Subject:   "test-sub",
		EntityID:  uuid.New().String(),
	}
}

func TestUnit_JobQueue_ListJobsPagination(t *testing.T) {
	ctx, s := store.SetupStore(t)

	var jobs []*store.Job
	var jobIDs []string

	totalJobs := 5
	for i := range totalJobs {
		job := newTestUnit_JobQueue_Job("list-paginate-test")
		err := s.AppendJob(ctx, *job)
		require.NoError(t, err)

		latestJobs, err := s.ListJobs(ctx, nil, 1)
		require.NoError(t, err)
		require.Len(t, latestJobs, 1)
		require.Equal(t, job.ID, latestJobs[0].ID, "Failed to retrieve job %s immediately after insertion", job.ID)

		job.CreatedAt = latestJobs[0].CreatedAt
		jobs = append(jobs, job)
		jobIDs = append(jobIDs, job.ID)

		if i < totalJobs-1 {
			time.Sleep(25 * time.Millisecond)
		}
	}

	// Order of insertion (and expected increasing CreatedAt): jobs[0], jobs[1], jobs[2], jobs[3], jobs[4]
	// Expected order from ListJobs (CreatedAt DESC): jobs[4], jobs[3], jobs[2], jobs[1], jobs[0]

	limit := 2

	t.Run("paginate_list_jobs_sequentially", func(t *testing.T) {
		var nextCursor *time.Time
		fetchedIDs := make([]string, 0, totalJobs)
		pageCount := 0

		for {
			pageCount++
			t.Logf("Fetching Page %d with cursor: %v", pageCount, nextCursor)

			currentPageJobs, err := s.ListJobs(ctx, nextCursor, limit)
			require.NoError(t, err, "Failed to list jobs for page %d", pageCount)

			if pageCount == 1 { // First page expectations
				require.NotEmpty(t, currentPageJobs, "First page should not be empty")
				require.Equal(t, jobIDs[4], currentPageJobs[0].ID, "Page 1, Item 1 should be newest (jobs[4])")
				if len(currentPageJobs) > 1 {
					require.Equal(t, jobIDs[3], currentPageJobs[1].ID, "Page 1, Item 2 should be second newest (jobs[3])")
					require.True(t, currentPageJobs[0].CreatedAt.After(currentPageJobs[1].CreatedAt) || currentPageJobs[0].CreatedAt.Equal(currentPageJobs[1].CreatedAt))
				}
			}

			if len(currentPageJobs) == 0 {
				t.Logf("Finished fetching pages after page %d", pageCount-1)
				break // No more items left
			}

			// Append fetched IDs
			for _, job := range currentPageJobs {
				if nextCursor != nil {
					require.True(t, job.CreatedAt.Equal(*nextCursor) || job.CreatedAt.Before(*nextCursor),
						"Job %s (CreatedAt %v) on page %d is newer than cursor %v", job.ID, job.CreatedAt, pageCount, *nextCursor)
				}
				fetchedIDs = append(fetchedIDs, job.ID)
			}

			if len(currentPageJobs) < limit {
				t.Logf("Reached last page (page %d) with %d items", pageCount, len(currentPageJobs))
				break
			}

			// Set cursor for the next iteration
			nextCursor = &currentPageJobs[len(currentPageJobs)-1].CreatedAt

			// Safety break to prevent infinite loops in case of test logic error
			require.LessOrEqual(t, pageCount, totalJobs, "Pagination seems stuck in a loop")
		}

		// Verify all jobs were fetched in the correct order
		expectedOrderIDs := []string{jobIDs[4], jobIDs[3], jobIDs[2], jobIDs[1], jobIDs[0]}
		require.Equal(t, expectedOrderIDs, fetchedIDs, "All jobs fetched page-by-page should match expected descending order")
	})

	t.Run("list_with_past_cursor", func(t *testing.T) {
		pastTime := jobs[0].CreatedAt.Add(-time.Hour) // Time definitely before the first job
		result, err := s.ListJobs(ctx, &pastTime, limit)
		require.NoError(t, err)
		require.Empty(t, result, "Should return no jobs for a cursor time before all jobs")
	})

	t.Run("list_with_limit_zero", func(t *testing.T) {
		result, err := s.ListJobs(ctx, nil, 0)
		require.NoError(t, err)
		require.Empty(t, result, "ListJobs with limit 0 should return no jobs")
	})
}

func TestUnit_JobQueue_LeasedJobLifecycle(t *testing.T) {
	ctx, s := store.SetupStore(t)

	// Create base job
	originalJob := &store.Job{
		ID:           uuid.New().String(),
		TaskType:     "leased-job-test",
		ScheduledFor: 1630000000,
		ValidUntil:   1630003600,
		RetryCount:   2,
		Payload:      []byte("{}"),
		CreatedAt:    time.Now().UTC(),
	}

	// TestUnit_JobQueue_ lease operations
	t.Run("full_lifecycle", func(t *testing.T) {
		// Append to leased jobs
		leaseDuration := 15 * time.Minute
		require.NoError(t, s.AppendLeasedJob(ctx, *originalJob, leaseDuration, "test-leaser"))

		// Verify lease metadata
		leasedJob, err := s.GetLeasedJob(ctx, originalJob.ID)
		require.NoError(t, err)
		require.Equal(t, "test-leaser", leasedJob.Leaser)
		require.WithinDuration(t, time.Now().UTC().Add(leaseDuration), leasedJob.LeaseExpiration, 1*time.Second)

		// List leased jobs
		jobs, err := s.ListLeasedJobs(ctx, nil, 10)
		require.NoError(t, err)
		require.Len(t, jobs, 1)

		// Delete leased job
		require.NoError(t, s.DeleteLeasedJob(ctx, originalJob.ID))
		_, err = s.GetLeasedJob(ctx, originalJob.ID)
		require.Error(t, err)
	})
}

func TestUnit_JobQueue_RetryCountPersistence(t *testing.T) {
	ctx, s := store.SetupStore(t)

	// Create job with retries
	job := &store.Job{
		ID:         uuid.New().String(),
		TaskType:   "retry-test",
		RetryCount: 3,
		Payload:    []byte("{}"),
	}

	// TestUnit_JobQueue_ retry count preservation
	require.NoError(t, s.AppendJob(ctx, *job))
	popped, err := s.PopJobForType(ctx, "retry-test")
	require.NoError(t, err)
	require.Equal(t, 3, popped.RetryCount)
}

func TestUnit_JobQueue_LeaseExpiration(t *testing.T) {
	ctx, s := store.SetupStore(t)

	// Create test job
	job := &store.Job{ID: uuid.New().String(), TaskType: "lease-expiration-test", Payload: []byte("{}")}

	// TestUnit_JobQueue_ lease duration calculation
	leaseDuration := 30 * time.Minute
	require.NoError(t, s.AppendLeasedJob(ctx, *job, leaseDuration, "lease-test"))

	leasedJob, err := s.GetLeasedJob(ctx, job.ID)
	require.NoError(t, err)

	expectedExpiration := time.Now().UTC().Add(leaseDuration)
	require.WithinDuration(t, expectedExpiration, leasedJob.LeaseExpiration, 1*time.Second,
		"Lease expiration should be set correctly")
}

func TestUnit_JobQueue_EmptyListOperations(t *testing.T) {
	ctx, s := store.SetupStore(t)
	now := time.Now().UTC()

	t.Run("empty_job_list", func(t *testing.T) {
		jobs, err := s.ListJobs(ctx, &now, 10)
		require.NoError(t, err)
		require.Empty(t, jobs)
	})

	t.Run("empty_leased_job_list", func(t *testing.T) {
		jobs, err := s.ListLeasedJobs(ctx, &now, 10)
		require.NoError(t, err)
		require.Empty(t, jobs)
	})
}

func TestUnit_JobQueue_PopNJobsForType(t *testing.T) {
	ctx, s := store.SetupStore(t)

	taskType := "batch-process"
	otherType := "other-process"

	// Setup test data
	jobs := []*store.Job{
		{
			ID:       uuid.New().String(),
			TaskType: taskType,
			Payload:  []byte(`{"batch": 1}`),
		},
		{
			ID:       uuid.New().String(),
			TaskType: taskType,
			Payload:  []byte(`{"batch": 2}`),
		},
		{
			ID:       uuid.New().String(),
			TaskType: taskType,
			Payload:  []byte(`{"batch": 3}`),
		},
		{
			ID:       uuid.New().String(),
			TaskType: otherType,
			Payload:  []byte(`{"other": 1}`),
		},
	}

	// Insert jobs with slight delay to ensure ordering
	for _, job := range jobs {
		require.NoError(t, s.AppendJob(ctx, *job))
		time.Sleep(10 * time.Millisecond)
	}

	t.Run("pop_fewer_than_available", func(t *testing.T) {
		// Pop 2 out of 3 available
		popped, err := s.PopNJobsForType(ctx, taskType, 2)
		require.NoError(t, err)
		require.Len(t, popped, 2)

		// Verify oldest jobs were popped first
		require.Equal(t, jobs[0].ID, popped[0].ID)
		require.Equal(t, jobs[1].ID, popped[1].ID)

		// Verify remaining jobs
		remaining, err := s.GetJobsForType(ctx, taskType)
		require.NoError(t, err)
		require.Len(t, remaining, 1)
		require.Equal(t, jobs[2].ID, remaining[0].ID)
	})

	t.Run("pop_more_than_available", func(t *testing.T) {
		// Now only 1 of our type remains (plus the other type)
		popped, err := s.PopNJobsForType(ctx, taskType, 5)
		require.NoError(t, err)
		require.Len(t, popped, 1)
		require.Equal(t, jobs[2].ID, popped[0].ID)

		// Verify queue is empty for our type
		remaining, err := s.GetJobsForType(ctx, taskType)
		require.NoError(t, err)
		require.Empty(t, remaining)
	})

	t.Run("pop_from_empty_queue", func(t *testing.T) {
		popped, err := s.PopNJobsForType(ctx, taskType, 1)
		require.NoError(t, err)
		require.Empty(t, popped)
	})

	t.Run("pop_exact_amount", func(t *testing.T) {
		// Reset state by adding 3 new jobs
		newJobs := []*store.Job{
			{
				ID:       uuid.New().String(),
				TaskType: taskType,
				Payload:  []byte(`{"new": 1}`),
			},
			{
				ID:       uuid.New().String(),
				TaskType: taskType,
				Payload:  []byte(`{"new": 2}`),
			},
			{
				ID:       uuid.New().String(),
				TaskType: taskType,
				Payload:  []byte(`{"new": 3}`),
			},
		}
		for _, job := range newJobs {
			require.NoError(t, s.AppendJob(ctx, *job))
			time.Sleep(10 * time.Millisecond)
		}

		// Pop exactly 3
		popped, err := s.PopNJobsForType(ctx, taskType, 3)
		require.NoError(t, err)
		require.Len(t, popped, 3)

		// Verify all were popped
		remaining, err := s.GetJobsForType(ctx, taskType)
		require.NoError(t, err)
		require.Empty(t, remaining)
	})

	t.Run("pop_zero_jobs", func(t *testing.T) {
		popped, err := s.PopNJobsForType(ctx, taskType, 0)
		require.NoError(t, err)
		require.Empty(t, popped)
	})

	t.Run("pop_negative_amount", func(t *testing.T) {
		popped, err := s.PopNJobsForType(ctx, taskType, -1)
		require.Error(t, err)
		require.Nil(t, popped)
	})

	t.Run("does_not_pop_other_types", func(t *testing.T) {
		// The otherType job should still be there
		popped, err := s.PopNJobsForType(ctx, otherType, 10)
		require.NoError(t, err)
		require.Len(t, popped, 1)
		require.Equal(t, jobs[3].ID, popped[0].ID)
	})
}

func TestUnit_JobQueue_DeleteJobsByEntity(t *testing.T) {
	ctx, s := store.SetupStore(t)

	entityID := uuid.New().String()
	entityType := "test-entity"

	// Insert jobs for the target entity
	for i := 0; i < 3; i++ {
		job := &store.Job{
			ID:         uuid.New().String(),
			TaskType:   "delete-by-entity",
			EntityID:   entityID,
			EntityType: entityType,
			Payload:    []byte(`{}`),
		}
		require.NoError(t, s.AppendJob(ctx, *job))
	}

	// Insert unrelated job
	otherJob := &store.Job{
		ID:         uuid.New().String(),
		TaskType:   "delete-by-entity",
		EntityID:   uuid.New().String(),
		EntityType: entityType,
		Payload:    []byte(`{}`),
	}
	require.NoError(t, s.AppendJob(ctx, *otherJob))

	// Delete by entity
	require.NoError(t, s.DeleteJobsByEntity(ctx, entityID, entityType))

	// Remaining jobs should only be the unrelated one
	jobs, err := s.GetJobsForType(ctx, "delete-by-entity")
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, otherJob.ID, jobs[0].ID)
}

func TestUnit_JobQueue_PopNJobsForType_EdgeCases(t *testing.T) {
	ctx, s := store.SetupStore(t)

	taskType := "edge-case-test"

	// Insert 2 jobs
	for i := 0; i < 2; i++ {
		job := &store.Job{
			ID:       uuid.New().String(),
			TaskType: taskType,
			Payload:  []byte(`{}`),
		}
		require.NoError(t, s.AppendJob(ctx, *job))
	}

	// Request more than available
	jobs, err := s.PopNJobsForType(ctx, taskType, 5)
	require.NoError(t, err)
	require.Len(t, jobs, 2)

	// Request zero
	jobs, err = s.PopNJobsForType(ctx, taskType, 0)
	require.NoError(t, err)
	require.Empty(t, jobs)

	// Request negative
	_, err = s.PopNJobsForType(ctx, taskType, -1)
	require.Error(t, err)
}
