package frr

import (
	"fmt"
	"os/exec"
	"strings"
)

type runner func(args ...string) ([]byte, error)

type Client struct {
	run runner
}

func NewClient() *Client {
	return &Client{run: func(args ...string) ([]byte, error) {
		return exec.Command("vtysh", args...).CombinedOutput()
	}}
}

func (c *Client) ApplyBase(b Base) error {
	if err := b.validate(); err != nil {
		return err
	}
	running, err := c.runningConfig()
	if err != nil {
		return err
	}
	stale, err := b.staleReflectors(running)
	if err != nil {
		return err
	}
	if err := c.configure(b.commands()...); err != nil {
		return err
	}
	if len(stale) > 0 {
		cmds := []string{fmt.Sprintf("router bgp %d", b.ASN)}
		for _, rr := range stale {
			cmds = append(cmds, "no neighbor "+rr)
		}
		if err := c.configure(append(cmds, "exit")...); err != nil {
			return err
		}
	}
	return c.save()
}

func (c *Client) ApplyVRF(v VRF) error {
	if err := v.validate(); err != nil {
		return err
	}
	if err := c.configure(v.commands()...); err != nil {
		return err
	}
	return c.save()
}

func (c *Client) WithdrawVRF(v VRF) error {
	return c.teardown(v, routingTeardown)
}

func (c *Client) RemoveVRF(v VRF) error {
	if err := c.teardown(v, vrfTeardown); err != nil {
		return err
	}
	running, err := c.runningConfig()
	if err != nil {
		return err
	}
	return remaining(routingTeardown(v, running))
}

func (c *Client) teardown(v VRF, plan func(VRF, string) []step) error {
	if err := v.validateName(); err != nil {
		return err
	}
	running, err := c.runningConfig()
	if err != nil {
		return err
	}
	for _, s := range plan(v, running) {
		if err := c.configure(s...); err != nil {
			return err
		}
	}
	if err := c.save(); err != nil {
		return err
	}
	if running, err = c.runningConfig(); err != nil {
		return err
	}
	return remaining(plan(v, running))
}

func (c *Client) runningConfig() (string, error) {
	return c.exec("show running-config")
}

func (c *Client) configure(cmds ...string) error {
	_, err := c.exec(append([]string{"configure terminal"}, cmds...)...)
	return err
}

func (c *Client) save() error {
	_, err := c.exec("write memory")
	return err
}

func (c *Client) exec(cmds ...string) (string, error) {
	args := make([]string, 0, 2*len(cmds))
	for _, cmd := range cmds {
		args = append(args, "-c", cmd)
	}
	out, err := c.run(args...)
	if err != nil {
		return "", fmt.Errorf("vtysh %q: %w: %s", cmds, err, strings.TrimSpace(string(out)))
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "%") {
			return "", fmt.Errorf("vtysh %q: %s", cmds, strings.TrimSpace(l))
		}
	}
	return string(out), nil
}
