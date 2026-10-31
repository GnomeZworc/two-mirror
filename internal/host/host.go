package host

import (
	"bufio"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

type CPU struct {
	Vendor  string
	Model   string
	Sockets int
	Cores   int
	Threads int
}

type OS struct {
	ID         string
	VersionID  string
	PrettyName string
}

type Facts struct {
	Hostname    string
	CPU         CPU
	MemoryBytes uint64
	OS          OS
	Kernel      string
}

type State struct {
	BootTime      int64
	UptimeSeconds float64
	Load          [3]float64
}

func ReadFacts(fsys fs.FS) (Facts, error) {
	var f Facts
	var err error
	if f.Hostname, err = readLine(fsys, "proc/sys/kernel/hostname"); err != nil {
		return Facts{}, err
	}
	if f.Kernel, err = readLine(fsys, "proc/sys/kernel/osrelease"); err != nil {
		return Facts{}, err
	}
	data, err := fs.ReadFile(fsys, "proc/cpuinfo")
	if err != nil {
		return Facts{}, err
	}
	if f.CPU, err = parseCPUInfo(string(data)); err != nil {
		return Facts{}, err
	}
	data, err = fs.ReadFile(fsys, "proc/meminfo")
	if err != nil {
		return Facts{}, err
	}
	if f.MemoryBytes, err = parseMemTotal(string(data)); err != nil {
		return Facts{}, err
	}
	data, err = fs.ReadFile(fsys, "etc/os-release")
	if err != nil {
		return Facts{}, err
	}
	f.OS = parseOSRelease(string(data))
	return f, nil
}

func ReadState(fsys fs.FS) (State, error) {
	var s State
	data, err := fs.ReadFile(fsys, "proc/stat")
	if err != nil {
		return State{}, err
	}
	if s.BootTime, err = parseBootTime(string(data)); err != nil {
		return State{}, err
	}
	line, err := readLine(fsys, "proc/uptime")
	if err != nil {
		return State{}, err
	}
	if s.UptimeSeconds, err = parseUptime(line); err != nil {
		return State{}, err
	}
	if line, err = readLine(fsys, "proc/loadavg"); err != nil {
		return State{}, err
	}
	if s.Load, err = parseLoadAvg(line); err != nil {
		return State{}, err
	}
	return s, nil
}

func readLine(fsys fs.FS, name string) (string, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		return "", fmt.Errorf("%s: empty", name)
	}
	return line, nil
}

func parseCPUInfo(data string) (CPU, error) {
	var cpu CPU
	sockets := map[string]bool{}
	cores := map[string]bool{}
	var physical string
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "processor":
			cpu.Threads++
			physical = ""
		case "vendor_id":
			cpu.Vendor = value
		case "model name":
			cpu.Model = value
		case "physical id":
			physical = value
			sockets[value] = true
		case "core id":
			if physical == "" {
				return CPU{}, fmt.Errorf("cpuinfo: core id %s without physical id", value)
			}
			cores[physical+"/"+value] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return CPU{}, fmt.Errorf("cpuinfo: %w", err)
	}
	if cpu.Threads == 0 || len(sockets) == 0 || len(cores) == 0 {
		return CPU{}, fmt.Errorf("cpuinfo: %d processors, %d physical ids, %d core ids", cpu.Threads, len(sockets), len(cores))
	}
	cpu.Sockets = len(sockets)
	cpu.Cores = len(cores)
	return cpu, nil
}

func parseMemTotal(data string) (uint64, error) {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != "MemTotal:" || fields[2] != "kB" {
			continue
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("meminfo: MemTotal: %w", err)
		}
		return kb * 1024, nil
	}
	return 0, fmt.Errorf("meminfo: no MemTotal in kB")
}

func parseOSRelease(data string) OS {
	var rel OS
	for _, line := range strings.Split(data, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		switch key {
		case "ID":
			rel.ID = value
		case "VERSION_ID":
			rel.VersionID = value
		case "PRETTY_NAME":
			rel.PrettyName = value
		}
	}
	return rel
}

func parseBootTime(data string) (int64, error) {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "btime" {
			t, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("stat: btime: %w", err)
			}
			return t, nil
		}
	}
	return 0, fmt.Errorf("stat: no btime")
}

func parseUptime(line string) (float64, error) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return 0, fmt.Errorf("uptime: %q", line)
	}
	up, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("uptime: %w", err)
	}
	return up, nil
}

func parseLoadAvg(line string) ([3]float64, error) {
	var load [3]float64
	fields := strings.Fields(line)
	if len(fields) != 5 {
		return load, fmt.Errorf("loadavg: %q", line)
	}
	for i := range load {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return load, fmt.Errorf("loadavg: %w", err)
		}
		load[i] = v
	}
	return load, nil
}
