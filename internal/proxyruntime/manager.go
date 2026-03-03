package proxyruntime

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	DefaultHost = "127.0.0.1"
	DefaultPort = 8317
)

type Manager struct {
	mu sync.RWMutex

	cliProxyPath string
	authDir      string

	host       string
	port       int
	proxyDir   string
	configPath string
	statePath  string

	cmd      *exec.Cmd
	waitDone chan struct{}
	stopping bool

	startedAt time.Time
	lastError string
	state     persistedState
}

type persistedState struct {
	APIKey        string `json:"apiKey"`
	ManagementKey string `json:"managementKey"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type Meta struct {
	ManagedConfigPath string
	ManagedStatePath  string
	Host              string
	Port              int
}

type PortConflict struct {
	Occupied bool   `json:"occupied"`
	PID      int    `json:"pid,omitempty"`
	Command  string `json:"command,omitempty"`
}

type Status struct {
	Running          bool          `json:"running"`
	PID              int           `json:"pid"`
	Host             string        `json:"host"`
	Port             int           `json:"port"`
	Endpoint         string        `json:"endpoint"`
	StartedAt        string        `json:"startedAt"`
	BinaryPath       string        `json:"binaryPath"`
	BinaryAccessible bool          `json:"binaryAccessible"`
	APIKeyMasked     string        `json:"apiKeyMasked"`
	LastError        string        `json:"lastError"`
	PortConflict     *PortConflict `json:"portConflict,omitempty"`
}

type Credentials struct {
	Endpoint     string `json:"endpoint"`
	APIKeyMasked string `json:"apiKeyMasked"`
	APIKeyPlain  string `json:"apiKeyPlain"`
	SampleEnv    string `json:"sampleEnv"`
}

type RotateResult struct {
	Status      Status `json:"status"`
	APIKeyPlain string `json:"apiKeyPlain"`
}

type PortConflictError struct {
	Conflict PortConflict
}

func (e *PortConflictError) Error() string {
	if e == nil {
		return "port conflict"
	}
	if e.Conflict.PID > 0 && e.Conflict.Command != "" {
		return fmt.Sprintf("proxy port %d is occupied by %s (pid %d)", DefaultPort, e.Conflict.Command, e.Conflict.PID)
	}
	return fmt.Sprintf("proxy port %d is occupied", DefaultPort)
}

func NewManager(cliProxyPath, authDir string) *Manager {
	home := safeHomeDir()
	proxyDir := filepath.Join(home, ".quotio-lite", "proxy")

	m := &Manager{
		cliProxyPath: cliProxyPath,
		authDir:      authDir,
		host:         DefaultHost,
		port:         DefaultPort,
		proxyDir:     proxyDir,
		configPath:   filepath.Join(proxyDir, "config.yaml"),
		statePath:    filepath.Join(proxyDir, "state.json"),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureStateAndConfigLocked(); err != nil {
		m.lastError = err.Error()
	}
	return m
}

func (m *Manager) Meta() Meta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Meta{
		ManagedConfigPath: m.configPath,
		ManagedStatePath:  m.statePath,
		Host:              m.host,
		Port:              m.port,
	}
}

func (m *Manager) Status(_ context.Context) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ensureStateAndConfigLocked(); err != nil {
		return Status{}, err
	}
	return m.statusLocked(), nil
}

func (m *Manager) Credentials(_ context.Context) (Credentials, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ensureStateAndConfigLocked(); err != nil {
		return Credentials{}, err
	}

	endpoint := m.endpointLocked()
	return Credentials{
		Endpoint:     endpoint,
		APIKeyMasked: maskAPIKey(m.state.APIKey),
		APIKeyPlain:  m.state.APIKey,
		SampleEnv: fmt.Sprintf(
			"OPENAI_BASE_URL=%s\nOPENAI_API_KEY=%s",
			endpoint,
			m.state.APIKey,
		),
	}, nil
}

func (m *Manager) Start(ctx context.Context) (Status, error) {
	m.mu.Lock()
	if m.runningLocked() {
		status := m.statusLocked()
		m.mu.Unlock()
		return status, nil
	}
	if err := m.ensureStateAndConfigLocked(); err != nil {
		m.mu.Unlock()
		return Status{}, err
	}
	if _, err := os.Stat(m.cliProxyPath); err != nil {
		m.mu.Unlock()
		return Status{}, fmt.Errorf("cli proxy unavailable: %w", err)
	}
	conflict := detectPortConflict(m.host, m.port)
	if conflict != nil {
		m.mu.Unlock()
		return Status{}, &PortConflictError{Conflict: *conflict}
	}

	cmd := exec.Command(m.cliProxyPath, "-config", m.configPath)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.mu.Unlock()
		return Status{}, fmt.Errorf("attach stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.mu.Unlock()
		return Status{}, fmt.Errorf("attach stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		return Status{}, fmt.Errorf("start cli proxy: %w", err)
	}

	done := make(chan struct{})
	m.cmd = cmd
	m.waitDone = done
	m.startedAt = time.Now().UTC()
	m.stopping = false
	m.lastError = ""
	m.mu.Unlock()

	go discardPipe(stdout)
	go discardPipe(stderr)
	go m.waitProcess(cmd, done)

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := waitForTCP(waitCtx, m.host, m.port); err != nil {
		_, _ = m.Stop(context.Background())
		return Status{}, fmt.Errorf("proxy startup timeout: %w", err)
	}
	return m.Status(ctx)
}

func (m *Manager) Stop(ctx context.Context) (Status, error) {
	m.mu.Lock()
	cmd := m.cmd
	done := m.waitDone
	if cmd == nil || cmd.Process == nil {
		status := m.statusLocked()
		m.mu.Unlock()
		return status, nil
	}
	m.stopping = true
	m.mu.Unlock()

	_ = cmd.Process.Signal(syscall.SIGTERM)
	if !waitForExit(ctx, done, 2*time.Second) {
		_ = cmd.Process.Kill()
		if !waitForExit(ctx, done, 2*time.Second) {
			return Status{}, errors.New("proxy process did not exit after force kill")
		}
	}
	return m.Status(ctx)
}

func (m *Manager) Restart(ctx context.Context) (Status, error) {
	if _, err := m.Stop(ctx); err != nil {
		return Status{}, err
	}
	return m.Start(ctx)
}

func (m *Manager) RotateAPIKey(ctx context.Context) (RotateResult, error) {
	m.mu.Lock()
	if err := m.ensureStateAndConfigLocked(); err != nil {
		m.mu.Unlock()
		return RotateResult{}, err
	}
	newKey, err := randomToken("sk-", 24)
	if err != nil {
		m.mu.Unlock()
		return RotateResult{}, fmt.Errorf("generate api key: %w", err)
	}
	m.state.APIKey = newKey
	m.state.UpdatedAt = nowRFC3339()
	if err := m.writeStateLocked(); err != nil {
		m.mu.Unlock()
		return RotateResult{}, err
	}
	if err := m.writeConfigLocked(); err != nil {
		m.mu.Unlock()
		return RotateResult{}, err
	}
	wasRunning := m.runningLocked()
	m.mu.Unlock()

	if wasRunning {
		if _, err := m.Restart(ctx); err != nil {
			return RotateResult{}, err
		}
	}
	status, err := m.Status(ctx)
	if err != nil {
		return RotateResult{}, err
	}
	return RotateResult{
		Status:      status,
		APIKeyPlain: newKey,
	}, nil
}

func (m *Manager) waitProcess(cmd *exec.Cmd, done chan struct{}) {
	err := cmd.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd == cmd {
		m.cmd = nil
		m.waitDone = nil
		m.startedAt = time.Time{}
	}
	if err != nil && !m.stopping {
		m.lastError = fmt.Sprintf("proxy exited: %v", err)
	}
	if err == nil && !m.stopping {
		m.lastError = "proxy exited"
	}
	m.stopping = false

	close(done)
}

func (m *Manager) ensureStateAndConfigLocked() error {
	if err := os.MkdirAll(m.proxyDir, 0o700); err != nil {
		return fmt.Errorf("create proxy runtime dir: %w", err)
	}

	if err := m.loadOrCreateStateLocked(); err != nil {
		return err
	}
	if err := m.writeConfigLocked(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) loadOrCreateStateLocked() error {
	raw, err := os.ReadFile(m.statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m.createStateLocked()
		}
		return fmt.Errorf("read proxy state: %w", err)
	}

	var state persistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("decode proxy state: %w", err)
	}
	updated := false

	if strings.TrimSpace(state.APIKey) == "" {
		token, genErr := randomToken("sk-", 24)
		if genErr != nil {
			return fmt.Errorf("generate api key: %w", genErr)
		}
		state.APIKey = token
		updated = true
	}
	if strings.TrimSpace(state.ManagementKey) == "" {
		token, genErr := randomToken("", 24)
		if genErr != nil {
			return fmt.Errorf("generate management key: %w", genErr)
		}
		state.ManagementKey = token
		updated = true
	}
	if strings.TrimSpace(state.CreatedAt) == "" {
		state.CreatedAt = nowRFC3339()
		updated = true
	}
	if strings.TrimSpace(state.UpdatedAt) == "" {
		state.UpdatedAt = nowRFC3339()
		updated = true
	}

	m.state = state
	if updated {
		m.state.UpdatedAt = nowRFC3339()
		return m.writeStateLocked()
	}
	return nil
}

func (m *Manager) createStateLocked() error {
	apiKey, err := randomToken("sk-", 24)
	if err != nil {
		return fmt.Errorf("generate api key: %w", err)
	}
	managementKey, err := randomToken("", 24)
	if err != nil {
		return fmt.Errorf("generate management key: %w", err)
	}
	now := nowRFC3339()
	m.state = persistedState{
		APIKey:        apiKey,
		ManagementKey: managementKey,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return m.writeStateLocked()
}

func (m *Manager) writeStateLocked() error {
	raw, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode proxy state: %w", err)
	}
	if err := os.WriteFile(m.statePath, raw, 0o600); err != nil {
		return fmt.Errorf("write proxy state: %w", err)
	}
	return nil
}

func (m *Manager) writeConfigLocked() error {
	content := fmt.Sprintf(`host: "%s"
port: %d
auth-dir: %q
api-keys:
  - %q
remote-management:
  allow-remote: false
  secret-key: %q
debug: false
logging-to-file: false
routing:
  strategy: "round-robin"
quota-exceeded:
  switch-project: true
  switch-preview-model: true
request-retry: 1
max-retry-interval: 5
`, m.host, m.port, m.authDir, m.state.APIKey, m.state.ManagementKey)

	if err := os.WriteFile(m.configPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write proxy config: %w", err)
	}
	return nil
}

func (m *Manager) statusLocked() Status {
	running := m.runningLocked()
	pid := 0
	if running && m.cmd != nil && m.cmd.Process != nil {
		pid = int(m.cmd.Process.Pid)
	}
	startedAt := ""
	if !m.startedAt.IsZero() {
		startedAt = m.startedAt.Format(time.RFC3339)
	}

	conflict := detectPortConflict(m.host, m.port)
	if running {
		conflict = nil
	}

	_, statErr := os.Stat(m.cliProxyPath)
	return Status{
		Running:          running,
		PID:              pid,
		Host:             m.host,
		Port:             m.port,
		Endpoint:         m.endpointLocked(),
		StartedAt:        startedAt,
		BinaryPath:       m.cliProxyPath,
		BinaryAccessible: statErr == nil,
		APIKeyMasked:     maskAPIKey(m.state.APIKey),
		LastError:        m.lastError,
		PortConflict:     conflict,
	}
}

func (m *Manager) runningLocked() bool {
	return m.cmd != nil && m.cmd.Process != nil && m.cmd.ProcessState == nil
}

func (m *Manager) endpointLocked() string {
	return fmt.Sprintf("http://%s:%d/v1", m.host, m.port)
}

func detectPortConflict(host string, port int) *PortConflict {
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err == nil {
		_ = listener.Close()
		return nil
	}

	conflict := &PortConflict{Occupied: true}
	path, lookErr := exec.LookPath("lsof")
	if lookErr != nil {
		return conflict
	}
	output, outErr := exec.Command(path, "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN").Output()
	if outErr != nil {
		return conflict
	}
	pid, command := parseLsofOutput(output)
	if pid > 0 {
		conflict.PID = pid
	}
	if command != "" {
		conflict.Command = command
	}
	return conflict
}

func parseLsofOutput(raw []byte) (int, string) {
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 2 {
		return 0, ""
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 2 {
		return 0, ""
	}
	pid, _ := strconv.Atoi(fields[1])
	return pid, fields[0]
}

func waitForTCP(ctx context.Context, host string, port int) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: 700 * time.Millisecond}

	for {
		conn, err := dialer.DialContext(ctx, "tcp", address)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForExit(ctx context.Context, done <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

func discardPipe(reader io.Reader) {
	_, _ = io.Copy(io.Discard, reader)
}

func safeHomeDir() string {
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return home
	}
	if env := strings.TrimSpace(os.Getenv("HOME")); env != "" {
		return env
	}
	return "."
}

func maskAPIKey(v string) string {
	key := strings.TrimSpace(v)
	if key == "" {
		return ""
	}
	if len(key) <= 10 {
		return key
	}
	return key[:7] + "********" + key[len(key)-4:]
}

func randomToken(prefix string, n int) (string, error) {
	if n <= 0 {
		n = 16
	}
	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(data), nil
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
