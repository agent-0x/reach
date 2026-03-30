package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/agent-0x/reach/internal/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(bootstrapCmd())
}

// bootstrapCmd — reach bootstrap <name> --host <ip> --user <user> [flags]
func bootstrapCmd() *cobra.Command {
	var (
		host      string
		user      string
		sshKey    string
		sshPort   int
		agentPort int
		agentDir  string
	)

	cmd := &cobra.Command{
		Use:   "bootstrap <name>",
		Short: "Deploy reach agent to a server via SSH (one-time setup)",
		Long: `Bootstrap deploys the reach agent to a remote server via SSH.
After bootstrap, SSH is no longer needed — use reach commands instead.

Steps performed:
  1. Test SSH connection
  2. Detect remote OS/arch
  3. Download reach binary from GitHub releases
  4. Upload and install binary
  5. Initialize agent (generate TLS cert + token)
  6. Install and start systemd/launchd service
  7. Register server locally (TOFU fingerprint)
  8. Verify health`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if host == "" {
				return fmt.Errorf("--host is required")
			}
			if user == "" {
				return fmt.Errorf("--user is required")
			}

			return runBootstrap(name, host, user, sshKey, sshPort, agentPort, agentDir)
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "server IP or hostname (required)")
	cmd.Flags().StringVar(&user, "user", "", "SSH username (required)")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "SSH private key path (default: use ssh-agent/default keys)")
	cmd.Flags().IntVar(&sshPort, "ssh-port", 22, "SSH port")
	cmd.Flags().IntVar(&agentPort, "agent-port", 7100, "reach agent port")
	cmd.Flags().StringVar(&agentDir, "agent-dir", "/etc/reach-agent", "agent config directory on remote")
	return cmd
}

// sshRun executes a command on the remote server via SSH.
// Returns trimmed stdout output.
func sshRun(host, user string, port int, keyPath string, command string) (string, error) {
	args := []string{}
	if keyPath != "" {
		args = append(args, "-i", keyPath)
	}
	args = append(args,
		"-p", fmt.Sprintf("%d", port),
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes",
		fmt.Sprintf("%s@%s", user, host),
		command,
	)
	cmd := exec.Command("ssh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// scpUpload copies a local file to the remote server via SCP.
func scpUpload(host, user string, port int, keyPath, localPath, remotePath string) error {
	args := []string{}
	if keyPath != "" {
		args = append(args, "-i", keyPath)
	}
	args = append(args,
		"-P", fmt.Sprintf("%d", port),
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		localPath,
		fmt.Sprintf("%s@%s:%s", user, host, remotePath),
	)
	cmd := exec.Command("scp", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// normalizeArch maps uname -m values to Go-style arch names.
func normalizeArch(arch string) string {
	switch arch {
	case "x86_64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return arch
	}
}

// fetchLatestTag queries GitHub API for the latest release tag of a repo.
// Returns the tag without the leading "v".
func fetchLatestTag(repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return "", fmt.Errorf("fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse release response: %w", err)
	}
	if result.TagName == "" {
		return "", fmt.Errorf("no tag_name in release response")
	}
	// Strip leading "v"
	tag := strings.TrimPrefix(result.TagName, "v")
	return tag, nil
}

// downloadFile downloads a URL to a local file path.
func downloadFile(url, destPath string) error {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", destPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write %s: %w", destPath, err)
	}
	return nil
}

// runBootstrap executes all bootstrap steps.
func runBootstrap(name, host, user, sshKey string, sshPort, agentPort int, agentDir string) error {
	step := func(msg string) {
		_, _ = fmt.Fprintf(os.Stderr, "→ %s\n", msg)
	}
	success := func(msg string) {
		_, _ = fmt.Fprintf(os.Stdout, "✓ %s\n", msg)
	}
	fail := func(step string, err error) error {
		return fmt.Errorf("bootstrap failed at [%s]: %w", step, err)
	}

	_, _ = fmt.Fprintf(os.Stderr, "\nBootstrapping reach agent on %s@%s...\n\n", user, host)

	// ── Step 1: Test SSH connection ────────────────────────────────────────
	step("Testing SSH connection...")
	if _, err := sshRun(host, user, sshPort, sshKey, "echo ok"); err != nil {
		return fail("SSH connection test", err)
	}
	success("SSH connection OK")

	// ── Step 2: Detect OS/arch ─────────────────────────────────────────────
	step("Detecting remote OS and architecture...")
	osName, err := sshRun(host, user, sshPort, sshKey, "uname -s | tr '[:upper:]' '[:lower:]'")
	if err != nil {
		return fail("detect OS", err)
	}
	rawArch, err := sshRun(host, user, sshPort, sshKey, "uname -m")
	if err != nil {
		return fail("detect arch", err)
	}
	arch := normalizeArch(rawArch)
	_, _ = fmt.Fprintf(os.Stderr, "  Remote: %s/%s\n", osName, arch)

	if osName != "linux" && osName != "darwin" {
		return fmt.Errorf("unsupported OS: %s (only linux/darwin supported)", osName)
	}
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("unsupported arch: %s (raw: %s)", arch, rawArch)
	}

	// ── Step 3: Check if reach already installed ───────────────────────────
	step("Checking if reach is already installed...")
	existingReach, _ := sshRun(host, user, sshPort, sshKey, "which reach 2>/dev/null || echo NOT_FOUND")
	if existingReach != "NOT_FOUND" && existingReach != "" {
		_, _ = fmt.Fprintf(os.Stderr, "  Warning: reach already found at %s — will overwrite.\n", existingReach)
	}

	// ── Step 4: Download binary from GitHub releases ───────────────────────
	step("Fetching latest reach release tag from GitHub...")
	tag, err := fetchLatestTag("agent-0x/reach")
	if err != nil {
		return fail("fetch latest tag", err)
	}
	_, _ = fmt.Fprintf(os.Stderr, "  Latest release: v%s\n", tag)

	tarName := fmt.Sprintf("reach_%s_%s_%s.tar.gz", tag, osName, arch)
	downloadURL := fmt.Sprintf("https://github.com/agent-0x/reach/releases/download/v%s/%s", tag, tarName)

	tmpDir, err := os.MkdirTemp("", "reach-bootstrap-*")
	if err != nil {
		return fail("create temp dir", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	localTar := tmpDir + "/reach.tar.gz"
	step(fmt.Sprintf("Downloading %s/%s binary...", osName, arch))
	if err := downloadFile(downloadURL, localTar); err != nil {
		return fail("download binary", err)
	}
	success(fmt.Sprintf("Downloaded reach v%s (%s/%s)", tag, osName, arch))

	// ── Step 5: Upload and install binary ─────────────────────────────────
	step("Uploading binary to remote server...")
	if err := scpUpload(host, user, sshPort, sshKey, localTar, "/tmp/reach.tar.gz"); err != nil {
		return fail("upload binary", err)
	}

	step("Installing binary to /usr/local/bin/reach...")
	installCmd := "tar xzf /tmp/reach.tar.gz -C /tmp && mv /tmp/reach /usr/local/bin/reach && chmod +x /usr/local/bin/reach && rm /tmp/reach.tar.gz"
	if _, err := sshRun(host, user, sshPort, sshKey, installCmd); err != nil {
		return fail("install binary", err)
	}
	success("Binary installed at /usr/local/bin/reach")

	// ── Step 6: Initialize agent ───────────────────────────────────────────
	step(fmt.Sprintf("Initializing reach agent (dir: %s)...", agentDir))
	initCmd := fmt.Sprintf("reach agent init --dir %s", agentDir)
	initOut, err := sshRun(host, user, sshPort, sshKey, initCmd)
	if err != nil {
		return fail("agent init", err)
	}
	success("Agent initialized")

	// ── Step 7: Extract token ──────────────────────────────────────────────
	step("Extracting agent token...")
	// Try to parse from init output first (line: "Token:  <value>")
	token := ""
	for _, line := range strings.Split(initOut, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Token:") {
			token = strings.TrimSpace(strings.TrimPrefix(line, "Token:"))
			break
		}
	}
	// Fallback: read from config file
	if token == "" {
		token, err = sshRun(host, user, sshPort, sshKey,
			fmt.Sprintf("grep 'token:' %s/config.yaml | awk '{print $2}'", agentDir))
		if err != nil {
			return fail("extract token", err)
		}
	}
	if token == "" {
		return fmt.Errorf("could not extract agent token from init output or config")
	}
	_, _ = fmt.Fprintf(os.Stderr, "  Token: %s\n", token)

	// ── Step 8: Install and start service ─────────────────────────────────
	step("Installing service manager unit...")
	if osName == "linux" {
		if err := installSystemdService(host, user, sshPort, sshKey, agentDir, agentPort); err != nil {
			return fail("install systemd service", err)
		}
		success("systemd service installed and started")
	} else {
		// macOS
		if err := installLaunchdService(host, user, sshPort, sshKey, agentDir, agentPort); err != nil {
			return fail("install launchd service", err)
		}
		success("launchd service installed and started")
	}

	// ── Step 9: Wait for agent to be ready ────────────────────────────────
	step("Waiting for agent to start...")
	time.Sleep(2 * time.Second)

	// ── Step 10: Register server locally (TOFU) ───────────────────────────
	step("Registering server in local config (TOFU)...")
	tmpCfg := &client.ServerConfig{
		Host:  host,
		Port:  agentPort,
		Token: token,
	}
	tmpClient := client.NewClient(name, tmpCfg)

	fp, err := tmpClient.GetFingerprint()
	if err != nil {
		return fail("get TLS fingerprint", fmt.Errorf("agent may not be ready yet: %w", err))
	}
	_, _ = fmt.Fprintf(os.Stderr, "  Fingerprint: sha256:%s\n", fp)

	verifiedCfg := &client.ServerConfig{
		Host:        host,
		Port:        agentPort,
		Token:       token,
		Fingerprint: fp,
	}
	verifiedClient := client.NewClient(name, verifiedCfg)

	// Verify health before saving
	ok, err := verifiedClient.Health()
	if err != nil {
		return fail("health check", err)
	}
	if !ok {
		return fmt.Errorf("agent health check returned not-OK")
	}

	// Save to local config
	cfg, err := client.LoadClientConfig()
	if err != nil {
		return fail("load local config", err)
	}
	cfg.Servers[name] = verifiedCfg
	if err := cfg.Save(); err != nil {
		return fail("save local config", err)
	}
	success(fmt.Sprintf("Server %q registered locally", name))

	// ── Done ───────────────────────────────────────────────────────────────
	_, _ = fmt.Fprintf(os.Stdout, "\n")
	success(fmt.Sprintf("reach agent deployed to %s (%s:%d)", name, host, agentPort))
	success("Agent registered locally")
	success("SSH is no longer needed for this server")
	_, _ = fmt.Fprintf(os.Stdout, "\n  reach exec %s \"uname -a\"    # try it!\n\n", name)

	return nil
}

// installSystemdService writes, enables and starts a systemd unit for the reach agent.
func installSystemdService(host, user string, port int, keyPath, agentDir string, agentPort int) error {
	unit := fmt.Sprintf(`[Unit]
Description=Reach Agent
After=network.target

[Service]
ExecStart=/usr/local/bin/reach agent serve --config %s/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target`, agentDir)

	// Write unit file via heredoc-style printf to avoid shell quoting issues
	writeUnitCmd := fmt.Sprintf(
		`printf '%%s' %s > /etc/systemd/system/reach-agent.service`,
		shellQuote(unit),
	)
	if _, err := sshRun(host, user, port, keyPath, writeUnitCmd); err != nil {
		// Fallback: use tee
		writeUnitCmd = fmt.Sprintf(
			`cat > /etc/systemd/system/reach-agent.service << 'REACH_UNIT'
%s
REACH_UNIT`, unit)
		if _, err2 := sshRun(host, user, port, keyPath, writeUnitCmd); err2 != nil {
			return fmt.Errorf("write unit file: %w", err2)
		}
	}

	if _, err := sshRun(host, user, port, keyPath,
		"systemctl daemon-reload && systemctl enable reach-agent && systemctl start reach-agent"); err != nil {
		return fmt.Errorf("enable/start service: %w", err)
	}
	return nil
}

// installLaunchdService writes a launchd plist and loads it for macOS.
func installLaunchdService(host, user string, port int, keyPath, agentDir string, agentPort int) error {
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.reach.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/reach</string>
        <string>agent</string>
        <string>serve</string>
        <string>--config</string>
        <string>%s/config.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>`, agentDir)

	plistPath := "/Library/LaunchDaemons/com.reach.agent.plist"
	writeCmd := fmt.Sprintf(
		`cat > %s << 'REACH_PLIST'
%s
REACH_PLIST`, plistPath, plist)
	if _, err := sshRun(host, user, port, keyPath, writeCmd); err != nil {
		return fmt.Errorf("write launchd plist: %w", err)
	}

	loadCmd := fmt.Sprintf("launchctl load -w %s", plistPath)
	if _, err := sshRun(host, user, port, keyPath, loadCmd); err != nil {
		return fmt.Errorf("load launchd service: %w", err)
	}
	return nil
}

// shellQuote wraps s in single quotes, escaping any single quotes within.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
