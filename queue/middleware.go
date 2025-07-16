package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/rs/zerolog/log"
)

// Middleware is a function that wraps a JobHandler
type Middleware func(JobHandler) JobHandler

// MiddlewareChain applies multiple middlewares to a job handler
func MiddlewareChain(handler JobHandler, middlewares ...Middleware) JobHandler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// LoggingMiddleware logs job execution
func LoggingMiddleware(next JobHandler) JobHandler {
	return func(ctx context.Context, job *Job) error {
		start := time.Now()

		log.Info().
			Str("job_id", job.ID).
			Str("job_type", job.Type).
			Str("priority", job.Priority.String()).
			Msg("job started")

		err := next(ctx, job)

		duration := time.Since(start)

		if err != nil {
			log.Error().
				Err(err).
				Str("job_id", job.ID).
				Str("job_type", job.Type).
				Dur("duration", duration).
				Msg("job failed")
		} else {
			log.Info().
				Str("job_id", job.ID).
				Str("job_type", job.Type).
				Dur("duration", duration).
				Msg("job completed")
		}

		return err
	}
}

// TimeoutMiddleware adds timeout to job execution
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next JobHandler) JobHandler {
		return func(ctx context.Context, job *Job) error {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- next(ctx, job)
			}()

			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				return apperror.NewError("job execution timeout")
			}
		}
	}
}

// MetricsMiddleware tracks job metrics
func MetricsMiddleware(next JobHandler) JobHandler {
	return func(ctx context.Context, job *Job) error {
		start := time.Now()

		// Increment job started counter
		log.Debug().
			Str("job_id", job.ID).
			Str("job_type", job.Type).
			Msg("job metrics: started")

		err := next(ctx, job)

		duration := time.Since(start)

		// Track duration and success/failure
		if err != nil {
			log.Debug().
				Str("job_id", job.ID).
				Str("job_type", job.Type).
				Dur("duration", duration).
				Msg("job metrics: failed")
		} else {
			log.Debug().
				Str("job_id", job.ID).
				Str("job_type", job.Type).
				Dur("duration", duration).
				Msg("job metrics: completed")
		}

		return err
	}
}

// RecoveryMiddleware recovers from panics in job handlers
func RecoveryMiddleware(next JobHandler) JobHandler {
	return func(ctx context.Context, job *Job) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = apperror.NewError(fmt.Sprintf("job panic: %v", r))
				log.Error().
					Str("job_id", job.ID).
					Str("job_type", job.Type).
					Interface("panic", r).
					Msg("job panic recovered")
			}
		}()

		return next(ctx, job)
	}
}
