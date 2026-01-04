package systemd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
)

const (
	defaultTimeout = 5 * time.Second
	jobMode        = "replace"
)

type Manager struct {
	conn *dbus.Conn
}

type ServiceStatus struct {
	Name        string
	LoadState   string
	ActiveState string
	SubState    string
	MainPID     uint32
}

// New crée une connexion D-Bus systemd (scope système)
func New() (*Manager, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return nil, err
	}

	return &Manager{conn: conn}, nil
}

// Close ferme la connexion D-Bus
func (m *Manager) Close() {
	if m.conn != nil {
		m.conn.Close()
	}
}

// Start démarre un service systemd
func (m *Manager) Start(service string) error {
	return m.job("StartUnit", service)
}

// Stop arrête un service systemd
func (m *Manager) Stop(service string) error {
	return m.job("StopUnit", service)
}

func (m *Manager) job(method, service string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	ch := make(chan string, 1)

	var err error
	switch method {
	case "StartUnit":
		_, err = m.conn.StartUnitContext(ctx, service, jobMode, ch)
	case "StopUnit":
		_, err = m.conn.StopUnitContext(ctx, service, jobMode, ch)
	default:
		return errors.New("unsupported job method")
	}

	if err != nil {
		return err
	}

	result := <-ch
	if result != "done" {
		return fmt.Errorf("%s %s failed: %s", method, service, result)
	}

	return nil
}

// Status retourne l’état courant du service
func (m *Manager) Status(service string) (*ServiceStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	props, err := m.conn.GetUnitPropertiesContext(ctx, service)
	if err != nil {
		return nil, err
	}

	status := &ServiceStatus{
		Name:        service,
		LoadState:   props["LoadState"].(string),
		ActiveState: props["ActiveState"].(string),
		SubState:    props["SubState"].(string),
	}

	if pid, ok := props["MainPID"].(uint32); ok {
		status.MainPID = pid
	}

	return status, nil
}
