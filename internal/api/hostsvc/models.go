package hostsvcapi

import "path/filepath"

type Verb string

const (
	VerbOpen  Verb = "open"
	VerbClose Verb = "close"
	VerbList  Verb = "list"
)

const (
	MaxMessageBytes = 64 * 1024
	DefaultRunDir   = "/run/two/hostsvc"
	Unit            = "two-hostsvc.service"
)

func SocketPath(runDir string) string { return filepath.Join(runDir, "hostsvc.sock") }
func StatePath(runDir string) string  { return filepath.Join(runDir, "hostsvc.state") }

type Request struct {
	Verb Verb   `json:"verb"`
	VPC  string `json:"vpc,omitempty"`
}

type Response struct {
	OK    bool     `json:"ok"`
	Error string   `json:"error,omitempty"`
	VPCs  []string `json:"vpcs,omitempty"`
}
