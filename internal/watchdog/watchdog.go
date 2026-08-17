package watchdog

import (
	"context"
	"log/slog"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/systemd"

	"github.com/dgraph-io/badger/v4"
)

const defaultInterval = 60 * time.Second

type Watchdog struct {
	db       *badger.DB
	cfg      *configuration.Config
	notifier notify.Notifier
	logger   *slog.Logger
	interval time.Duration
	dbusDown bool
}

func New(db *badger.DB, cfg *configuration.Config, n notify.Notifier, logger *slog.Logger, interval time.Duration) *Watchdog {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		logger.Warn("watchdog: intervalle invalide, valeur par défaut appliquée",
			"interval", interval, "default", defaultInterval)
		interval = defaultInterval
	}
	return &Watchdog{
		db:       db,
		cfg:      cfg,
		notifier: n,
		logger:   logger,
		interval: interval,
	}
}

func (w *Watchdog) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.logger.Info("watchdog: démarrage", "interval", w.interval)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("watchdog: arrêt")
			return
		case <-ticker.C:
			w.tick()
		}
	}
}

func (w *Watchdog) tick() {
	u, closeUnits := w.units()
	defer closeUnits()

	if err := CheckVPCs(w.db, w.notifier); err != nil {
		w.logger.Error("watchdog: vérification des vpc", "err", err)
	}
	if err := CheckSubnets(w.db, u, w.notifier); err != nil {
		w.logger.Error("watchdog: vérification des subnets", "err", err)
	}
	if err := CheckVMs(w.db, w.cfg, u, w.notifier); err != nil {
		w.logger.Error("watchdog: vérification des vm", "err", err)
	}
}

func (w *Watchdog) units() (unitChecker, func()) {
	m, err := systemd.New()
	if err != nil {
		if !w.dbusDown {
			w.logger.Warn("watchdog: connexion systemd impossible, vérification des units désactivée", "err", err)
			w.dbusDown = true
		}
		return nil, func() {}
	}
	if w.dbusDown {
		w.logger.Info("watchdog: connexion systemd rétablie")
		w.dbusDown = false
	}
	return m, m.Close
}
