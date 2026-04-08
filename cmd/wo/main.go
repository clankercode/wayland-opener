package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xertrov/wo/internal/handler"
	"github.com/xertrov/wo/internal/ipc"
	"github.com/xertrov/wo/internal/launch"
	"github.com/xertrov/wo/internal/session"
)

func main() {
	start := time.Now()
	defer func() {
		fmt.Fprintf(os.Stderr, "[wo] %.3fms\n", time.Since(start).Seconds()*1000)
	}()
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "wo — wayland opener\n\n")
		fmt.Fprintf(os.Stderr, "Usage: wo [options] <file|url>\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -l, --list          List available Wayland sessions\n")
		fmt.Fprintf(os.Stderr, "  -s, --session NAME  Target specific Wayland session (e.g. wayland-0)\n")
		fmt.Fprintf(os.Stderr, "  -d, --dry-run       Show what would be launched without launching\n")
		fmt.Fprintf(os.Stderr, "  -a, --app CMD       Use specific app instead of auto-detecting\n")
		fmt.Fprintf(os.Stderr, "  -n, --no-daemon     Launch directly without daemon\n")
		fmt.Fprintf(os.Stderr, "  -h, --help          Show this help\n")
		os.Exit(1)
	}

	var (
		listSessions bool
		sessionName  string
		dryRun       bool
		appOverride  string
		noDaemon     bool
		targets      []string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-l", "--list":
			listSessions = true
		case "-s", "--session":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --session requires an argument")
				os.Exit(1)
			}
			sessionName = args[i]
		case "-d", "--dry-run":
			dryRun = true
		case "-a", "--app":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --app requires an argument")
				os.Exit(1)
			}
			appOverride = args[i]
		case "-n", "--no-daemon":
			noDaemon = true
		case "-h", "--help":
			fmt.Fprintf(os.Stderr, "wo — wayland opener\n\nUsage: wo [options] <file|url|->\n")
			os.Exit(0)
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "unknown option: %s\n", args[i])
				os.Exit(1)
			}
			targets = append(targets, args[i])
		}
	}

	if !noDaemon {
		if err := ipc.EnsureDaemon(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			fmt.Fprintf(os.Stderr, "(try wo -n to launch without daemon)\n")
			os.Exit(1)
		}
	}

	if listSessions {
		if noDaemon {
			listSessionsDirect()
		} else {
			listSessionsDaemon()
		}
		return
	}

	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "error: no target specified")
		os.Exit(1)
	}

	for _, target := range targets {
		if noDaemon {
			openDirect(target, sessionName, appOverride, dryRun)
		} else {
			openViaDaemon(target, sessionName, appOverride, dryRun)
		}
	}
}

func listSessionsDaemon() {
	resp, err := ipc.SendRequest(ipc.Request{Action: "list"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "(try wo -l -n for direct discovery)\n")
		os.Exit(1)
	}
	if !resp.OK {
		fmt.Fprintf(os.Stderr, "error: %s\n", resp.Error)
		os.Exit(1)
	}
	if len(resp.Sessions) == 0 {
		fmt.Println("No Wayland sessions found")
		return
	}
	fmt.Printf("%-15s %-15s %-10s %s\n", "WAYLAND_DISPLAY", "DESKTOP", "SEAT", "ACTIVE")
	for _, s := range resp.Sessions {
		fmt.Printf("%-15s %-15s %-10s %s\n", s.WaylandDisplay, s.Desktop, s.Seat, strconv.FormatBool(s.Active))
	}
}

func listSessionsDirect() {
	sessions, err := session.Discover()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Println("No Wayland sessions found")
		return
	}
	fmt.Printf("%-15s %-15s %-10s %s\n", "WAYLAND_DISPLAY", "DESKTOP", "SEAT", "ACTIVE")
	for _, s := range sessions {
		fmt.Printf("%-15s %-15s %-10s %s\n", s.WaylandDisplay, s.Desktop, s.Seat, strconv.FormatBool(s.Active))
	}
}

func openViaDaemon(target, sessionName, appOverride string, dryRun bool) {
	req := ipc.Request{
		Action:  "open",
		Target:  target,
		Session: sessionName,
	}
	resp, err := ipc.SendRequest(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "(try wo -n to launch without daemon)\n")
		os.Exit(1)
	}
	if !resp.OK {
		fmt.Fprintf(os.Stderr, "error: %s\n", resp.Error)
		os.Exit(1)
	}
	if dryRun {
		fmt.Printf("would open: %s\n", resp.Command)
	} else {
		fmt.Printf("opened: %s\n", resp.Command)
	}
}

func openDirect(target, sessionName, appOverride string, dryRun bool) {
	sessions, err := session.Discover()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error discovering sessions: %v\n", err)
		os.Exit(1)
	}

	var targetSession *session.Session
	for i := range sessions {
		if sessionName != "" && sessions[i].WaylandDisplay == sessionName {
			s := sessions[i]
			targetSession = &s
			break
		}
	}
	if targetSession == nil {
		if cur := session.Current(); cur != nil {
			targetSession = cur
		} else if len(sessions) > 0 {
			s := sessions[0]
			targetSession = &s
		}
	}
	if targetSession == nil {
		fmt.Fprintln(os.Stderr, "error: no Wayland session found")
		os.Exit(1)
	}

	cmd := appOverride
	term := false
	if cmd == "" {
		h, err := handler.FindHandler(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		cmd = handler.PrepareCommand(h.Exec, target)
		term = h.Terminal
	} else {
		cmd = cmd + " " + handler.PrepareCommand("", target)
	}

	termCmd := ""
	if term {
		termCmd = detectTerminal()
	}

	err = launch.Launch(target, launch.Options{
		Session:      targetSession,
		App:          cmd,
		Terminal:     term,
		TerminalCmd:  termCmd,
		DryRun:       dryRun,
		SystemdScope: false,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if dryRun {
		fmt.Printf("would open: %s on %s\n", target, targetSession.WaylandDisplay)
	} else {
		fmt.Printf("opened: %s on %s\n", target, targetSession.WaylandDisplay)
	}
}

func detectTerminal() string {
	for _, term := range []string{"foot", "kitty", "alacritty", "wezterm", "ghostty", "weston-terminal"} {
		if _, err := os.Stat("/usr/bin/" + term); err == nil {
			return term
		}
	}
	return "xterm"
}
