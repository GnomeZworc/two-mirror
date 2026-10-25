package hostsvcclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	hostsvcapi "git.g3e.fr/syonad/two/internal/api/hostsvc"
)

const DefaultTimeout = 5 * time.Second

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

func (c *Client) call(req hostsvcapi.Request) (hostsvcapi.Response, error) {
	conn, err := net.DialTimeout("unix", c.path, c.timeout)
	if err != nil {
		return hostsvcapi.Response{}, fmt.Errorf("dial %s: %w", c.path, err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return hostsvcapi.Response{}, fmt.Errorf("set deadline on %s: %w", c.path, err)
	}

	raw, err := json.Marshal(req)
	if err != nil {
		return hostsvcapi.Response{}, fmt.Errorf("encode %s: %w", req.Verb, err)
	}
	if _, err := conn.Write(append(raw, '\n')); err != nil {
		return hostsvcapi.Response{}, fmt.Errorf("send %s: %w", req.Verb, err)
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), hostsvcapi.MaxMessageBytes)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return hostsvcapi.Response{}, fmt.Errorf("read reply to %s: %w", req.Verb, err)
		}
		return hostsvcapi.Response{}, fmt.Errorf("no reply to %s", req.Verb)
	}

	var resp hostsvcapi.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return hostsvcapi.Response{}, fmt.Errorf("parse reply to %s: %w", req.Verb, err)
	}
	if !resp.OK {
		return resp, fmt.Errorf("%s refused: %s", req.Verb, resp.Error)
	}
	return resp, nil
}

func (c *Client) Open(vpc string) error {
	_, err := c.call(hostsvcapi.Request{Verb: hostsvcapi.VerbOpen, VPC: vpc})
	return err
}

func (c *Client) Close(vpc string) error {
	_, err := c.call(hostsvcapi.Request{Verb: hostsvcapi.VerbClose, VPC: vpc})
	return err
}

func (c *Client) List() ([]string, error) {
	resp, err := c.call(hostsvcapi.Request{Verb: hostsvcapi.VerbList})
	if err != nil {
		return nil, err
	}
	return resp.VPCs, nil
}
