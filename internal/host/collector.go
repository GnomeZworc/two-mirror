package host

import (
	"context"
	"io/fs"
	"log/slog"
	"sync"
	"time"
)

type Ping struct {
	State
	AgentVersion   string
	AgentStartTime int64
	CollectedAt    int64
}

type Collector struct {
	fsys     fs.FS
	version  string
	start    time.Time
	interval time.Duration
	logger   *slog.Logger
	now      func() time.Time
	facts    Facts
	factsErr error

	mu   sync.RWMutex
	last *Ping
}

func NewCollector(fsys fs.FS, version string, start time.Time, interval time.Duration, logger *slog.Logger) *Collector {
	c := &Collector{fsys: fsys, version: version, start: start, interval: interval, logger: logger, now: time.Now}
	c.facts, c.factsErr = ReadFacts(fsys)
	if c.factsErr != nil {
		logger.Error("host facts unavailable", "error", c.factsErr)
	}
	return c
}

func (c *Collector) Register() (Facts, error) {
	return c.facts, c.factsErr
}

func (c *Collector) Ping() (Ping, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.last == nil {
		return Ping{}, false
	}
	return *c.last, true
}

func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	c.collect()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

func (c *Collector) collect() {
	s, err := ReadState(c.fsys)
	if err != nil {
		c.logger.Error("host state collection failed", "error", err)
		return
	}
	p := Ping{State: s, AgentVersion: c.version, AgentStartTime: c.start.Unix(), CollectedAt: c.now().Unix()}
	c.mu.Lock()
	c.last = &p
	c.mu.Unlock()
}
