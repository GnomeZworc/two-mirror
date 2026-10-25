package nft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

const Table = "two"

type hook struct {
	name     string
	chain    string
	prefix   string
	ports    string
	key      string
	priority string
}

var hooks = []hook{
	{"prerouting", "prerouting", "pre-", "pre_ports", "iifname", "dstnat"},
	{"forward", "forward", "fwd-", "fwd_ports", "oifname", "filter"},
	{"output", "output", "out-", "out_ports", "oifname", "filter"},
}

type Port struct {
	Name       string
	Prerouting []string
	Forward    []string
	Output     []string
}

func (p Port) rules(h hook) []string {
	switch h.name {
	case "prerouting":
		return p.Prerouting
	case "forward":
		return p.Forward
	default:
		return p.Output
	}
}

func DropSourceMAC(mac string) string {
	return "ether saddr " + mac + " drop"
}

func DropARPRequestTo(ip string) string {
	return "arp operation request arp daddr ip " + ip + " drop"
}

func DropARPFrom(ip string) string {
	return "arp saddr ip " + ip + " drop"
}

func DropDHCPFrom(ip string) string {
	return "ip saddr " + ip + " udp sport 67-68 udp dport 67-68 drop"
}

func run(args []string, script string) ([]byte, error) {
	cmd := exec.Command("nft", args...)
	if script != "" {
		cmd.Stdin = strings.NewReader(script)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nft %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func apply(script string) error {
	_, err := run([]string{"-f", "-"}, script)
	return err
}

func baseScript() string {
	var b strings.Builder
	fmt.Fprintf(&b, "add table bridge %s\n", Table)
	for _, h := range hooks {
		fmt.Fprintf(&b, "add map bridge %s %s { type ifname : verdict; }\n", Table, h.ports)
	}
	for _, h := range hooks {
		fmt.Fprintf(&b, "add chain bridge %s %s { type filter hook %s priority %s; policy accept; }\n", Table, h.chain, h.name, h.priority)
	}
	return b.String()
}

func setScript(p Port, base table) string {
	var b strings.Builder
	b.WriteString(baseScript())
	for _, h := range hooks {
		if !base.hasDispatch(h) {
			fmt.Fprintf(&b, "add rule bridge %s %s %s vmap @%s\n", Table, h.chain, h.key, h.ports)
		}
	}
	for _, h := range hooks {
		rules := p.rules(h)
		chain := h.prefix + p.Name
		if len(rules) == 0 {
			if _, ok := base.elements[h.ports][p.Name]; ok {
				fmt.Fprintf(&b, "delete element bridge %s %s { %q }\n", Table, h.ports, p.Name)
			}
			if base.chains[chain] {
				fmt.Fprintf(&b, "delete chain bridge %s %s\n", Table, chain)
			}
			continue
		}
		fmt.Fprintf(&b, "add chain bridge %s %s\n", Table, chain)
		fmt.Fprintf(&b, "flush chain bridge %s %s\n", Table, chain)
		for _, r := range rules {
			fmt.Fprintf(&b, "add rule bridge %s %s %s\n", Table, chain, r)
		}
		target, ok := base.elements[h.ports][p.Name]
		if ok && target != chain {
			fmt.Fprintf(&b, "delete element bridge %s %s { %q }\n", Table, h.ports, p.Name)
		}
		if target != chain {
			fmt.Fprintf(&b, "add element bridge %s %s { %q : jump %s }\n", Table, h.ports, p.Name, chain)
		}
	}
	return b.String()
}

func deleteScript(name string, t table) string {
	var b strings.Builder
	for _, h := range hooks {
		if _, ok := t.elements[h.ports][name]; ok {
			fmt.Fprintf(&b, "delete element bridge %s %s { %q }\n", Table, h.ports, name)
		}
	}
	for _, h := range hooks {
		if chain := h.prefix + name; t.chains[chain] {
			fmt.Fprintf(&b, "delete chain bridge %s %s\n", Table, chain)
		}
	}
	return b.String()
}

func SetPort(p Port) error {
	t, err := readTable()
	if err != nil {
		return err
	}
	if err := apply(setScript(p, t)); err != nil {
		return fmt.Errorf("set port %s: %w", p.Name, err)
	}
	return nil
}

func DeletePort(name string) error {
	t, err := readTable()
	if err != nil {
		return err
	}
	script := deleteScript(name, t)
	if script == "" {
		return nil
	}
	if err := apply(script); err != nil {
		return fmt.Errorf("delete port %s: %w", name, err)
	}
	return nil
}

func MissingRules(p Port) ([]string, error) {
	t, err := readTable()
	if err != nil {
		return nil, err
	}
	listings := map[string][]string{}
	for _, h := range hooks {
		chain := h.prefix + p.Name
		if len(p.rules(h)) == 0 || !t.chains[chain] {
			continue
		}
		out, err := run([]string{"list", "chain", "bridge", Table, chain}, "")
		if err != nil {
			return nil, err
		}
		listings[chain] = chainRules(string(out))
	}
	return missing(p, t, listings), nil
}

func missing(p Port, t table, listings map[string][]string) []string {
	var out []string
	for _, h := range hooks {
		rules := p.rules(h)
		if len(rules) == 0 {
			continue
		}
		chain := h.prefix + p.Name
		if !t.hasDispatch(h) {
			out = append(out, fmt.Sprintf("%s: %s vmap @%s", h.chain, h.key, h.ports))
		}
		if t.elements[h.ports][p.Name] != chain {
			out = append(out, fmt.Sprintf("%s: %q : jump %s", h.ports, p.Name, chain))
		}
		want := map[string]bool{}
		for _, r := range rules {
			want[r] = true
		}
		have := map[string]bool{}
		for _, r := range listings[chain] {
			have[r] = true
			if !want[r] {
				out = append(out, chain+": unexpected "+r)
			}
		}
		for _, r := range rules {
			if !have[r] {
				out = append(out, chain+": "+r)
			}
		}
	}
	return out
}

func chainRules(listing string) []string {
	var rules []string
	depth := 0
	for _, line := range strings.Split(listing, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasSuffix(line, "{"):
			depth++
		case line == "}":
			depth--
		case depth == 2 && line != "":
			rules = append(rules, line)
		}
	}
	return rules
}

type table struct {
	exists   bool
	chains   map[string]bool
	elements map[string]map[string]string
	dispatch map[string]string
}

func (t table) hasDispatch(h hook) bool {
	return t.dispatch[h.chain] == h.key+" @"+h.ports
}

type listing struct {
	Nftables []struct {
		Table *struct {
			Family string `json:"family"`
			Name   string `json:"name"`
		} `json:"table"`
		Chain *struct {
			Name string `json:"name"`
		} `json:"chain"`
		Map *struct {
			Name string            `json:"name"`
			Elem []json.RawMessage `json:"elem"`
		} `json:"map"`
		Rule *struct {
			Chain string `json:"chain"`
			Expr  []struct {
				Vmap *struct {
					Key struct {
						Meta struct {
							Key string `json:"key"`
						} `json:"meta"`
					} `json:"key"`
					Data string `json:"data"`
				} `json:"vmap"`
			} `json:"expr"`
		} `json:"rule"`
	} `json:"nftables"`
}

func readTable() (table, error) {
	out, err := run([]string{"-j", "list", "tables", "bridge"}, "")
	if err != nil {
		return table{}, err
	}
	tables, err := parseListing(out)
	if err != nil || !tables.exists {
		return tables, err
	}
	out, err = run([]string{"-j", "list", "table", "bridge", Table}, "")
	if err != nil {
		return table{}, err
	}
	return parseListing(out)
}

func parseListing(data []byte) (table, error) {
	var l listing
	if err := json.Unmarshal(data, &l); err != nil {
		return table{}, fmt.Errorf("nft json: %w", err)
	}
	t := table{chains: map[string]bool{}, elements: map[string]map[string]string{}, dispatch: map[string]string{}}
	for _, o := range l.Nftables {
		switch {
		case o.Table != nil:
			if o.Table.Family == "bridge" && o.Table.Name == Table {
				t.exists = true
			}
		case o.Chain != nil:
			t.chains[o.Chain.Name] = true
		case o.Map != nil:
			elems := map[string]string{}
			for _, raw := range o.Map.Elem {
				name, target, err := parseElem(raw)
				if err != nil {
					return table{}, fmt.Errorf("nft json: map %s: %w", o.Map.Name, err)
				}
				elems[name] = target
			}
			t.elements[o.Map.Name] = elems
		case o.Rule != nil:
			for _, e := range o.Rule.Expr {
				if e.Vmap != nil {
					t.dispatch[o.Rule.Chain] = e.Vmap.Key.Meta.Key + " " + e.Vmap.Data
				}
			}
		}
	}
	return t, nil
}

func parseElem(raw json.RawMessage) (string, string, error) {
	var pair []json.RawMessage
	if err := json.Unmarshal(raw, &pair); err != nil || len(pair) != 2 {
		return "", "", fmt.Errorf("element %s: not a key and verdict pair", raw)
	}
	var name string
	if err := json.Unmarshal(pair[0], &name); err != nil {
		return "", "", fmt.Errorf("element %s: key: %w", raw, err)
	}
	var verdict struct {
		Jump *struct {
			Target string `json:"target"`
		} `json:"jump"`
	}
	if err := json.Unmarshal(pair[1], &verdict); err != nil {
		return "", "", fmt.Errorf("element %s: verdict: %w", raw, err)
	}
	if verdict.Jump == nil {
		return name, "", nil
	}
	return name, verdict.Jump.Target, nil
}
