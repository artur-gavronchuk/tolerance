package competitions

import (
	"context"
	"log/slog"
	"time"
)

// RunCloser closes expired competitions every `every` until ctx is done.
// The UPDATE is atomic, so several api instances may run this safely.
func RunCloser(ctx context.Context, s *Service, every time.Duration, log *slog.Logger) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := s.CloseExpired(ctx)
			if err != nil {
				log.Error("close expired competitions", "err", err)
			} else if n > 0 {
				log.Info("closed expired competitions", "count", n)
			}
		}
	}
}
