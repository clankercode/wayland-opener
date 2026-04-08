package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/xertrov/wo/internal/handler"
	"github.com/xertrov/wo/internal/ipc"
	"github.com/xertrov/wo/internal/launch"
	"github.com/xertrov/wo/internal/session"
)

var (
	sessions []session.Session
	mu       sync.RWMutex
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("wo daemon starting...")

	refreshSessions()

	l, err := ipc.Listen()
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer l.Close()

	go refreshLoop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		<-sigCh
		log.Println("shutting down...")
		os.Remove(ipc.SocketPath())
		os.Exit(0)
	}()

	log.Printf("wo daemon listening on %s", ipc.SocketPath())

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handleConn(conn)
	}
}

func refreshLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		refreshSessions()
	}
}

func refreshSessions() {
	mu.Lock()
	defer mu.Unlock()
	s, err := session.Discover()
	if err != nil {
		log.Printf("session discovery: %v", err)
		return
	}
	sessions = s
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	req, err := ipc.ReadRequest(conn)
	if err != nil {
		ipc.WriteResponse(conn, ipc.Response{OK: false, Error: err.Error()})
		return
	}

	switch req.Action {
	case "list":
		handleList(conn)
	case "open":
		handleOpen(conn, req)
	default:
		ipc.WriteResponse(conn, ipc.Response{OK: false, Error: fmt.Sprintf("unknown action: %s", req.Action)})
	}
}

func handleList(conn net.Conn) {
	mu.RLock()
	defer mu.RUnlock()

	var infos []ipc.SessionInfo
	for _, s := range sessions {
		infos = append(infos, ipc.SessionInfo{
			WaylandDisplay: s.WaylandDisplay,
			Desktop:        s.Desktop,
			Seat:           s.Seat,
			Active:         s.Active,
		})
	}

	ipc.WriteResponse(conn, ipc.Response{OK: true, Sessions: infos})
}

func handleOpen(conn net.Conn, req *ipc.Request) {
	mu.RLock()
	var targetSession *session.Session
	for i := range sessions {
		if req.Session != "" && sessions[i].WaylandDisplay == req.Session {
			targetSession = &sessions[i]
			break
		}
	}
	mu.RUnlock()

	if targetSession == nil {
		mu.RLock()
		for i := range sessions {
			if sessions[i].Active {
				s := sessions[i]
				targetSession = &s
				break
			}
		}
		mu.RUnlock()
	}

	if targetSession == nil {
		ipc.WriteResponse(conn, ipc.Response{OK: false, Error: "no active Wayland session found"})
		return
	}

	h, err := handler.FindHandler(req.Target)
	if err != nil {
		ipc.WriteResponse(conn, ipc.Response{OK: false, Error: err.Error()})
		return
	}

	cmd := handler.PrepareCommand(h.Exec, req.Target)

	termCmd := ""
	if h.Terminal {
		termCmd = detectTerminal()
	}

	opts := launch.Options{
		Session:      targetSession,
		App:          cmd,
		Terminal:     h.Terminal,
		TerminalCmd:  termCmd,
		SystemdScope: true,
	}

	err = launch.Launch(req.Target, opts)
	if err != nil {
		ipc.WriteResponse(conn, ipc.Response{OK: false, Error: err.Error()})
		return
	}

	ipc.WriteResponse(conn, ipc.Response{
		OK:      true,
		Command: fmt.Sprintf("%s (%s on %s)", h.Name, req.Target, targetSession.WaylandDisplay),
	})
}

func detectTerminal() string {
	for _, term := range []string{"foot", "kitty", "alacritty", "wezterm", "ghostty", "weston-terminal"} {
		if _, err := os.Stat("/usr/bin/" + term); err == nil {
			return term
		}
		if p, _ := findInPath(term); p != "" {
			return p
		}
	}
	return "xterm"
}

func findInPath(name string) (string, error) {
	path := os.Getenv("PATH")
	for _, dir := range splitPath(path) {
		candidate := dir + "/" + name
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func splitPath(p string) []string {
	// json import needed for something else? let's just split
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == ':' {
			result = append(result, p[start:i])
			start = i + 1
		}
	}
	result = append(result, p[start:])
	return result
}
