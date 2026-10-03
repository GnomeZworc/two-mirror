package topology

import (
	"fmt"
	"io"
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
