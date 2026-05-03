package receiver

import "context"

// Receiver is anything that produces events into the unified out channel.
// Implementations should be non-blocking on out (drop or buffer rather than
// stalling ingestion) and respect ctx cancellation for shutdown.
type Receiver interface {
	Start(ctx context.Context, out chan<- any) error
	Stop() error
	Name() string
}
