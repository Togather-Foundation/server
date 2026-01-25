package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// AlertFunc is invoked when a job fails or panics.
type AlertFunc func(ctx context.Context, job *rivertype.JobRow, err error)

// AlertingErrorHandler logs and forwards job failures for alerting.
type AlertingErrorHandler struct {
	Logger *slog.Logger
	Notify AlertFunc
}

// NewAlertingErrorHandler builds an ErrorHandler that logs and forwards errors.
func NewAlertingErrorHandler(logger *slog.Logger, notify AlertFunc) *AlertingErrorHandler {
	return &AlertingErrorHandler{
		Logger: logger,
		Notify: notify,
	}
}

func (h *AlertingErrorHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	if h.Logger != nil {
		h.Logger.Error("job failed", "job_id", job.ID, "kind", job.Kind, "attempt", job.Attempt, "error", err)
	}
	if h.Notify != nil {
		h.Notify(ctx, job, err)
	}
	return nil
}

func (h *AlertingErrorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	panicErr := fmt.Errorf("panic: %v", panicVal)
	if h.Logger != nil {
		h.Logger.Error("job panicked", "job_id", job.ID, "kind", job.Kind, "attempt", job.Attempt, "error", panicErr, "trace", trace)
	}
	if h.Notify != nil {
		h.Notify(ctx, job, panicErr)
	}
	return nil
}
