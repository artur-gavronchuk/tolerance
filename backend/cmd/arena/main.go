// Command arena is the connector an agent owner runs on their own machine.
// It keeps the agent online, receives proof tasks, runs the owner's agent
// command locally and sends back only the resulting diff.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "login":
		err = cmdLogin()
	case "init":
		err = cmdInit()
	case "connect":
		err = cmdConnect()
	case "status":
		err = cmdStatus()
	case "tanks":
		err = runTanks(os.Args[2:], os.Stdout, os.Stderr)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "arena:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: arena <login|init|connect|status|tanks>

  login       read an API key from stdin and store it in ~/.arena/key
  init        write ~/.arena/config.yaml with the agent command to edit
  connect     stay online and run proof tasks as they arrive
  status      show the agent's stage and latest proof (not a heartbeat)
  tanks new   scaffold a starter tanks bot: arena tanks new <dir> [--lang python|js]
  tanks play  play a local tanks match: arena tanks play <bot>... [--seed N] [--map NAME] [--ticks N] [--out FILE]`)
}

func cmdLogin() error {
	dir, err := arenaDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	fmt.Fprint(os.Stderr, "Paste the API key: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	key := strings.TrimSpace(line)
	if !strings.HasPrefix(key, "ak_") {
		return errors.New("that does not look like an API key (expected ak_…)")
	}
	if err := os.WriteFile(filepath.Join(dir, "key"), []byte(key+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Println("Key saved to", filepath.Join(dir, "key"))
	return nil
}

func cmdInit() error {
	dir, err := arenaDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; edit it instead", path)
	}
	url := os.Getenv("ARENA_URL")
	if url == "" {
		url = "https://arena.example.com"
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf(defaultConfig, url)), 0o600); err != nil {
		return err
	}
	fmt.Println("Wrote", path, "— set agent.command to how your agent is started.")
	return nil
}

func newClient() (*client, config, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, cfg, err
	}
	key, err := loadKey()
	if err != nil {
		return nil, cfg, err
	}
	return &client{base: cfg.URL, key: key, http: &http.Client{Timeout: 60 * time.Second}}, cfg, nil
}

func cmdStatus() error {
	c, _, err := newClient()
	if err != nil {
		return err
	}
	st, err := c.Status(context.Background())
	if err != nil {
		return err
	}
	fmt.Print(formatStatus(st, time.Now()))
	return nil
}

// formatStatus renders what `arena status` prints.
func formatStatus(st statusResp, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n", st.Agent.Name, st.Agent.Stage)
	p := st.LastProof
	if p == nil {
		b.WriteString("last proof: none yet\n")
		return b.String()
	}
	fmt.Fprintf(&b, "last proof: %s %s", p.TaskSlug, p.Status)
	if p.FailureReason != "" {
		fmt.Fprintf(&b, " (%s)", p.FailureReason)
	}
	fmt.Fprintf(&b, ", started %s ago\n", now.Sub(p.CreatedAt).Round(time.Second))
	return b.String()
}

func cmdConnect() error {
	c, cfg, err := newClient()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	hb, err := c.Heartbeat(ctx)
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	fmt.Printf("%s is online (%s). Waiting for tasks; Ctrl-C to stop.\n", hb.Agent.Name, hb.Agent.Stage)

	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := c.Heartbeat(ctx); err != nil {
					fmt.Fprintln(os.Stderr, "heartbeat:", err)
				}
			}
		}
	}()

	backoff := time.Second
	for ctx.Err() == nil {
		task, err := c.NextTask(ctx, 25*time.Second)
		if err != nil {
			var ae *apiError
			if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized {
				return errors.New("API key rejected; run `arena login` with a fresh key")
			}
			fmt.Fprintln(os.Stderr, "poll:", err)
			select {
			case <-ctx.Done():
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = time.Second
		if task == nil {
			continue
		}
		fmt.Printf("Task %s (%s): running your agent, up to %ds\n", task.Task.Slug, task.ProofID, task.Task.AgentTimeoutS)
		// The server gives up on this proof agent_timeout_s + 60s after the
		// claim, so nothing is worth retrying past that.
		taskCtx, cancel := context.WithTimeout(ctx, time.Duration(task.Task.AgentTimeoutS+60)*time.Second)
		repo, err := c.Repo(taskCtx, task.ProofID)
		if err != nil {
			cancel()
			fmt.Fprintln(os.Stderr, "download repo:", err)
			continue
		}
		_ = c.Started(taskCtx, task.ProofID)
		res, err := runTask(taskCtx, *task, repo, cfg.Agent.Command)
		if err != nil {
			fmt.Fprintln(os.Stderr, "run:", err)
			res = result{LogTail: "connector error: " + err.Error(), ExitCode: -1}
		}
		err = c.Result(taskCtx, task.ProofID, res)
		cancel()
		if err != nil {
			fmt.Fprintln(os.Stderr, "send result:", err)
			continue
		}
		fmt.Printf("Result sent (exit %d, %d bytes of diff). Check the dashboard for the verdict.\n", res.ExitCode, len(res.Diff))
	}
	return nil
}
