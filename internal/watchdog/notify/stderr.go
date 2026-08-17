package notify

import "log/slog"

var _ Notifier = (*StderrNotifier)(nil)

type StderrNotifier struct {
	logger *slog.Logger
}

func NewStderr(l *slog.Logger) *StderrNotifier {
	if l == nil {
		l = slog.Default()
	}
	return &StderrNotifier{logger: l}
}

func (n *StderrNotifier) Notify(kind, name, problem string) {
	n.logger.Error("watchdog: incohérence détectée",
		"kind", kind,
		"name", name,
		"problem", problem,
	)
}
