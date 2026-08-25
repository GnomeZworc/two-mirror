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
	jobTimeout     = 30 * time.Second
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
	conn, err := dbus.NewSystemConnectionContext(context.Background())
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

// Restart redémarre un service systemd
func (m *Manager) Restart(service string) error {
	return m.job("RestartUnit", service)
}

func (m *Manager) job(method, service string) error {
	callCtx, callCancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer callCancel()

	ch := make(chan string, 1)

	var err error
	switch method {
	case "StartUnit":
		_, err = m.conn.StartUnitContext(callCtx, service, jobMode, ch)
	case "StopUnit":
		_, err = m.conn.StopUnitContext(callCtx, service, jobMode, ch)
	case "RestartUnit":
		_, err = m.conn.RestartUnitContext(callCtx, service, jobMode, ch)
	default:
		return errors.New("unsupported job method")
	}

	if err != nil {
		return err
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), jobTimeout)
	defer waitCancel()

	select {
	case result := <-ch:
		if result != "done" {
			return fmt.Errorf("%s %s failed: %s", method, service, result)
		}
	case <-waitCtx.Done():
		return fmt.Errorf("%s %s timed out after %s", method, service, jobTimeout)
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

	status := &ServiceStatus{Name: service}

	if v, ok := props["LoadState"].(string); ok {
		status.LoadState = v
	}
	if v, ok := props["ActiveState"].(string); ok {
		status.ActiveState = v
	}
	if v, ok := props["SubState"].(string); ok {
		status.SubState = v
	}
	if pid, ok := props["MainPID"].(uint32); ok {
		status.MainPID = pid
	}

	return status, nil
}
