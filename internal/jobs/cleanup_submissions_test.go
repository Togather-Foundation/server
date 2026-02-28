package jobs

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func newCleanupJob() *river.Job[SubmissionsCleanupArgs] {
	return &river.Job[SubmissionsCleanupArgs]{
		JobRow: &rivertype.JobRow{
			Kind:    JobKindSubmissionsCleanup,
			Attempt: 1,
		},
		Args: SubmissionsCleanupArgs{},
	}
}

func TestSubmissionsCleanupArgs_Kind(t *testing.T) {
	args := SubmissionsCleanupArgs{}
	if args.Kind() != JobKindSubmissionsCleanup {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindSubmissionsCleanup)
	}
}

func TestSubmissionsCleanupWorker_NilPool(t *testing.T) {
	t.Parallel()
	w := SubmissionsCleanupWorker{Pool: nil}
	err := w.Work(context.Background(), newCleanupJob())
	if err == nil {
		t.Fatal("expected error when Pool is nil")
	}
}
