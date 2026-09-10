package dhcpclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	dhcpapi "git.g3e.fr/syonad/two/internal/api/dhcp"
)

const DefaultTimeout = 5 * time.Second

var ErrNotServed = errors.New("mac is not served by this subnet")

type Client struct {
	path    string
	timeout time.Duration
}

func New(path string) *Client {
	return &Client{path: path, timeout: DefaultTimeout}
}

func (c *Client) WithTimeout(d time.Duration) *Client {
	return &Client{path: c.path, timeout: d}
}

func (c *Client) call(req dhcpapi.Request) (dhcpapi.Response, error) {
	conn, err := net.DialTimeout("unix", c.path, c.timeout)
	if err != nil {
		return dhcpapi.Response{}, fmt.Errorf("dial %s: %w", c.path, err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return dhcpapi.Response{}, fmt.Errorf("set deadline on %s: %w", c.path, err)
	}

	raw, err := json.Marshal(req)
	if err != nil {
		return dhcpapi.Response{}, fmt.Errorf("encode %s: %w", req.Verb, err)
	}
	if _, err := conn.Write(append(raw, '\n')); err != nil {
		return dhcpapi.Response{}, fmt.Errorf("send %s: %w", req.Verb, err)
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), dhcpapi.MaxMessageBytes)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return dhcpapi.Response{}, fmt.Errorf("read reply to %s: %w", req.Verb, err)
		}
		return dhcpapi.Response{}, fmt.Errorf("no reply to %s", req.Verb)
	}

	var resp dhcpapi.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return dhcpapi.Response{}, fmt.Errorf("parse reply to %s: %w", req.Verb, err)
	}
	if !resp.OK {
		return resp, fmt.Errorf("%s refused: %s", req.Verb, resp.Error)
	}
	return resp, nil
}

func (c *Client) SetSubnet(subnet dhcpapi.Subnet) error {
	_, err := c.call(dhcpapi.Request{Verb: dhcpapi.VerbSetSubnet, Subnet: &subnet})
	return err
}

func (c *Client) SetHost(host dhcpapi.Host) error {
	_, err := c.call(dhcpapi.Request{Verb: dhcpapi.VerbSetHost, Host: &host})
	return err
}

func (c *Client) DelHost(mac string) error {
	_, err := c.call(dhcpapi.Request{Verb: dhcpapi.VerbDelHost, MAC: mac})
	return err
}

func (c *Client) GetState() (dhcpapi.State, string, error) {
	resp, err := c.call(dhcpapi.Request{Verb: dhcpapi.VerbGetState})
	if err != nil {
		return dhcpapi.State{}, "", err
	}
	if resp.State == nil {
		return dhcpapi.State{}, "", errors.New("get-state returned no state")
	}
	return *resp.State, resp.Digest, nil
}

func (c *Client) Probe(mac string) (dhcpapi.Lease, error) {
	resp, err := c.call(dhcpapi.Request{Verb: dhcpapi.VerbProbe, MAC: mac})
	if err != nil {
		return dhcpapi.Lease{}, err
	}
	if !resp.Served || resp.Lease == nil {
		return dhcpapi.Lease{}, ErrNotServed
	}
	return *resp.Lease, nil
}
