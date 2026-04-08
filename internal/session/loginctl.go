package session

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type loginctlSession struct {
	ID             string
	User           string
	Seat           string
	Type           string
	Desktop        string
	Display        string
	Active         bool
	WaylandDisplay string
}

func getLoginctlSessions() ([]loginctlSession, error) {
	cmd := exec.Command("loginctl", "list-sessions", "--no-legend", "--no-pager")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var sessions []loginctlSession
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		sessionID := fields[0]
		user := fields[2]

		ls := loginctlSession{
			ID:   sessionID,
			User: user,
		}

		showCmd := exec.Command("loginctl", "show-session", sessionID,
			"-p", "Type", "-p", "Desktop", "-p", "Display", "-p", "Active", "-p", "Seat")
		showOut, err := showCmd.Output()
		if err != nil {
			continue
		}

		for _, propLine := range strings.Split(string(showOut), "\n") {
			propLine = strings.TrimSpace(propLine)
			kv := strings.SplitN(propLine, "=", 2)
			if len(kv) != 2 {
				continue
			}
			switch kv[0] {
			case "Type":
				ls.Type = kv[1]
			case "Desktop":
				ls.Desktop = kv[1]
			case "Display":
				ls.Display = kv[1]
			case "Active":
				ls.Active = kv[1] == "yes"
			case "Seat":
				ls.Seat = kv[1]
			}
		}

		if ls.Type == "wayland" {
			ls.WaylandDisplay = findWaylandDisplayForSession(sessionID)
			sessions = append(sessions, ls)
		}
	}

	return sessions, nil
}

func findWaylandDisplayForSession(sessionID string) string {
	for i := 0; i < 20; i++ {
		socketName := fmt.Sprintf("wayland-%d", i)
		if socketExists(socketName) {
			pid := getCompositorPIDForSocket(socketName)
			if pid != 0 {
				if belongsToSession(pid, sessionID) {
					return socketName
				}
			}
		}
	}
	return ""
}

func socketExists(name string) bool {
	runtimeDir := getRuntimeDir()
	path := runtimeDir + "/" + name
	info, err := exec.Command("stat", path).CombinedOutput()
	_ = info
	return err == nil
}

func getCompositorPIDForSocket(socketName string) uint32 {
	runtimeDir := getRuntimeDir()
	socketPath := runtimeDir + "/" + socketName

	cmd := exec.Command("fuser", socketPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	pidStr := strings.TrimSpace(string(out))
	fields := strings.Fields(pidStr)
	if len(fields) == 0 {
		return 0
	}

	pid, err := strconv.ParseUint(fields[0], 10, 32)
	if err != nil {
		return 0
	}
	return uint32(pid)
}

func belongsToSession(pid uint32, sessionID string) bool {
	sessionNum := strings.TrimPrefix(sessionID, "session")
	_ = sessionNum
	return true
}
