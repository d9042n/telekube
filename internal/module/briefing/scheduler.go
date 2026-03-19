package briefing

import (
	"context"
	"fmt"
	"time"

	"github.com/d9042n/telekube/internal/module/watcher"
	"go.uber.org/zap"
)

// Scheduler manages cron-based execution of briefing reports.
type Scheduler struct {
	reporter    *Reporter
	notifier    watcher.Notifier
	cfg         Config
	chats       []int64
	logger      *zap.Logger
	done        chan struct{}
}

// NewScheduler creates a new briefing scheduler.
func NewScheduler(
	reporter *Reporter,
	notifier watcher.Notifier,
	cfg Config,
	chats []int64,
	logger *zap.Logger,
) *Scheduler {
	return &Scheduler{
		reporter: reporter,
		notifier: notifier,
		cfg:      cfg,
		chats:    chats,
		logger:   logger,
		done:     make(chan struct{}),
	}
}

// Start begins the scheduler loop.
func (s *Scheduler) Start(ctx context.Context) {
	go s.run(ctx)
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

// run is the main scheduler loop.
func (s *Scheduler) run(ctx context.Context) {
	// Parse timezone
	loc, err := time.LoadLocation(s.cfg.Timezone)
	if err != nil {
		s.logger.Warn("invalid timezone, using UTC",
			zap.String("timezone", s.cfg.Timezone),
			zap.Error(err),
		)
		loc = time.UTC
	}

	s.logger.Info("briefing scheduler running",
		zap.String("schedule", s.cfg.Schedule),
		zap.String("timezone", loc.String()),
	)

	// Simple cron implementation: check every minute if it's time to run.
	// For a production-grade cron, we'd use robfig/cron, but to avoid
	// adding a dependency, we implement a simpler daily schedule check.
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	var lastRun time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case now := <-ticker.C:
			nowLocal := now.In(loc)

			// Parse schedule "0 8 * * *" => HH:MM
			hour, minute := parseCronTime(s.cfg.Schedule)

			if nowLocal.Hour() == hour && nowLocal.Minute() == minute {
				// Don't run more than once per minute window
				if lastRun.IsZero() || time.Since(lastRun) > 2*time.Minute {
					lastRun = time.Now()
					s.executeBriefing(ctx)
				}
			}
		}
	}
}

// executeBriefing generates and sends the briefing report.
func (s *Scheduler) executeBriefing(ctx context.Context) {
	s.logger.Info("generating daily briefing")

	report, err := s.reporter.Generate(ctx)
	if err != nil {
		s.logger.Error("failed to generate briefing", zap.Error(err))
		return
	}

	text := report.Format()

	for _, chatID := range s.chats {
		if err := s.notifier.SendAlert(chatID, text, nil); err != nil {
			s.logger.Error("failed to send briefing",
				zap.Int64("chat_id", chatID),
				zap.Error(err),
			)
		}
	}

	s.logger.Info("daily briefing sent",
		zap.Int("chats", len(s.chats)),
	)
}

// parseCronTime parses a simple cron expression for minute and hour.
// Supports: "M H * * *" format (daily at H:M).
func parseCronTime(schedule string) (hour, minute int) {
	// Default: 8:00
	hour = 8
	minute = 0

	var m, h int
	n, _ := fmt.Sscanf(schedule, "%d %d", &m, &h)
	if n >= 2 {
		hour = h
		minute = m
	}
	return
}
