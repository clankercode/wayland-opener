package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Session struct {
	WaylandDisplay string
	Display        string
	Desktop        string
	Seat           string
	CompositorPID  uint32
	CompositorCmd  string
	Active         bool
}

func Discover() ([]Session, error) {
	var sessions []Session

	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}

	waylandSockets, err := findWaylandSockets(runtimeDir)
	if err != nil {
		return nil, fmt.Errorf("scanning wayland sockets: %w", err)
	}

	loginctlSessions, err := getLoginctlSessions()
	if err == nil {
		for _, ls := range loginctlSessions {
			for _, socket := range waylandSockets {
				if ls.WaylandDisplay == socket {
					sessions = append(sessions, Session{
						WaylandDisplay: socket,
						Display:        ls.Display,
						Desktop:        ls.Desktop,
						Seat:           ls.Seat,
						Active:         ls.Active,
					})
					break
				}
			}
		}
	}

	if len(sessions) == 0 {
		for _, socket := range waylandSockets {
			sessions = append(sessions, Session{
				WaylandDisplay: socket,
			})
		}
	}

	return sessions, nil
}

func findWaylandSockets(runtimeDir string) ([]string, error) {
	var sockets []string

	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "wayland-") && !strings.HasSuffix(name, ".lock") {
			fullPath := filepath.Join(runtimeDir, name)
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSocket != 0 {
				sockets = append(sockets, name)
			}
			_ = fullPath
		}
	}

	return sockets, nil
}

func (s *Session) Env() []string {
	env := []string{
		fmt.Sprintf("WAYLAND_DISPLAY=%s", s.WaylandDisplay),
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	env = append(env, fmt.Sprintf("XDG_RUNTIME_DIR=%s", runtimeDir))
	if s.Display != "" {
		env = append(env, fmt.Sprintf("DISPLAY=%s", s.Display))
	}
	return env
}

func Current() *Session {
	wd := os.Getenv("WAYLAND_DISPLAY")
	if wd == "" {
		return nil
	}
	return &Session{
		WaylandDisplay: wd,
		Display:        os.Getenv("DISPLAY"),
		Desktop:        os.Getenv("XDG_CURRENT_DESKTOP"),
		Active:         true,
	}
}
