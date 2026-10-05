package topology

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
)

func (p *Plan) Write(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintf(tw, "lab %s: nodes %d, segments %d, cables %d\n", p.Name, len(p.Nodes), len(p.Segments), len(p.Cables))

	fmt.Fprintf(tw, "\nnodes\n")
	fmt.Fprintf(tw, "  name\trole\timage\tcpus\tmemory\tssh\n")
	for _, n := range p.Nodes {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%d\t%d MiB\t127.0.0.1:%d\n", n.Name, n.Role, n.Image, n.CPUs, n.Memory, n.SSHPort)
	}

	var extras []NodePlan
	for _, n := range p.Nodes {
		if len(n.Secondary) > 0 || n.Loopback.IsValid() || n.FRR != "" || n.Release != "" || n.Agent != "" {
			extras = append(extras, n)
		}
	}
	if len(extras) > 0 {
		fmt.Fprintf(tw, "\nroles\n")
		fmt.Fprintf(tw, "  name\tloopback\tsecondary\tfrr\trelease\tagent\n")
		for _, n := range extras {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\n", n.Name, orDash(loopback(n)), orDash(secondary(n)), orDash(filepath.Base(n.FRR)), orDash(n.Release), orDash(filepath.Base(n.Agent)))
		}
	}

	for _, s := range p.Segments {
		fmt.Fprintf(tw, "\nsegment %s: %s, mtu %d, switch %s, bridge %s, gateway %s\n",
			s.Name, s.Network, s.MTU, s.Switch, s.Bridge, s.Gateway)
		fmt.Fprintf(tw, "  node\tinterface\taddress\tmac\tudp\t\tswitch port\tmac\tudp\n")
		for _, c := range p.Cables {
			if c.Segment != s.Name {
				continue
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%d\t<->\t%s %s\t%s\t%d\n",
				c.Node, c.NodeInterface, c.NodeAddress, c.NodeMAC, c.NodePort,
				c.Switch, c.SwitchInterface, c.SwitchMAC, c.SwitchPort)
		}
	}
	return tw.Flush()
}

func loopback(n NodePlan) string {
	if !n.Loopback.IsValid() {
		return ""
	}
	return LoopbackInterface + " " + n.Loopback.String()
}

func secondary(n NodePlan) string {
	segments := make([]string, 0, len(n.Secondary))
	for s := range n.Secondary {
		segments = append(segments, s)
	}
	sort.Strings(segments)
	var parts []string
	for _, s := range segments {
		for _, prefix := range n.Secondary[s] {
			parts = append(parts, s+" "+prefix.String())
		}
	}
	return strings.Join(parts, ", ")
}

func orDash(s string) string {
	if s == "" || s == "." {
		return "-"
	}
	return s
}
