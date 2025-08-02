package dispatchservice

import (
	"context"
	"fmt"
	"time"

	"github.com/contenox/activitytracker"
	"github.com/contenox/runtime/store"
)

type activityTrackerDecorator struct {
	service Service
	tracker activitytracker.ActivityTracker
}

func (d *activityTrackerDecorator) PendingJobs(ctx context.Context, createdAtCursor *time.Time) ([]*store.Job, error) {
	reportErrFn, _, endFn := d.tracker.Start(ctx, "list", "pending-jobs")
	defer endFn()

	jobs, err := d.service.PendingJobs(ctx, createdAtCursor)
	if err != nil {
		reportErrFn(err)
	}

	return jobs, err
}

func (d *activityTrackerDecorator) InProgressJobs(ctx context.Context, createdAtCursor *time.Time) ([]*store.LeasedJob, error) {
	reportErrFn, _, endFn := d.tracker.Start(ctx, "list", "in-progress-jobs")
	defer endFn()

	jobs, err := d.service.InProgressJobs(ctx, createdAtCursor)
	if err != nil {
		reportErrFn(err)
	}

	return jobs, err
}

func (d *activityTrackerDecorator) AssignPendingJob(ctx context.Context, leaserID string, leaseDuration *time.Duration, jobTypes ...string) (*store.LeasedJob, error) {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"assign",
		"job",
		"leaserID", leaserID,
		"leaseDuration", leaseDuration,
		"jobTypes", fmt.Sprintf("%v", jobTypes),
	)
	defer endFn()

	job, err := d.service.AssignPendingJob(ctx, leaserID, leaseDuration, jobTypes...)
	if err != nil {
		reportErrFn(err)
	} else if job != nil {
		reportChangeFn(job.ID, map[string]any{
			"leaser":          job.Leaser,
			"leaseExpiration": job.LeaseExpiration,
			"taskType":        job.TaskType,
			"scheduledFor":    job.ScheduledFor,
		})
	}

	return job, err
}

func (d *activityTrackerDecorator) MarkJobAsDone(ctx context.Context, jobID string, leaserID string) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"complete",
		"job",
		"jobID", jobID,
		"leaserID", leaserID,
	)
	defer endFn()

	err := d.service.MarkJobAsDone(ctx, jobID, leaserID)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(jobID, nil)
	}

	return err
}

func (d *activityTrackerDecorator) MarkJobAsFailed(ctx context.Context, jobID string, leaserID string) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"fail",
		"job",
		"jobID", jobID,
		"leaserID", leaserID,
	)
	defer endFn()

	err := d.service.MarkJobAsFailed(ctx, jobID, leaserID)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(jobID, map[string]any{
			"retried": true,
		})
	}

	return err
}

func WithActivityTracker(service Service, tracker activitytracker.ActivityTracker) Service {
	return &activityTrackerDecorator{
		service: service,
		tracker: tracker,
	}
}

var _ Service = (*activityTrackerDecorator)(nil)
