package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"maactl/internal/pi"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

const (
	// agentConnectTimeout bounds the wait for an AgentServer to connect and
	// register its custom recognitions and actions. A slow agent still gets the
	// whole timeout, while a stuck one fails with a clear error instead of
	// hanging maactl forever.
	agentConnectTimeout = 60 * time.Second
	// agentExitGrace is how long an agent child process may take to exit on its
	// own after the AgentClient disconnected or the process was killed.
	agentExitGrace = 3 * time.Second

	// agentLogTerm and agentLogOff are the reserved --agent-log values; every
	// other value is a directory for per-agent log files.
	agentLogTerm = "term"
	agentLogOff  = "off"
)

// agentProcess owns one ProjectInterface agent: the child process running the
// AgentServer and the AgentClient connecting maactl to it.
//
// Agents start only after the selected resource has been loaded, because the
// client is bound to that resource before connecting; the custom recognitions
// and actions the agent registers then belong to the resource the task runs on.
type agentProcess struct {
	client *maa.AgentClient
	cmd    *exec.Cmd
	logs   *agentLogs
	// exitErr holds cmd.Wait's result and is read only after exited is closed.
	exitErr error
	exited  chan struct{}
}

// agentLaunch holds everything needed to start one of the declared agents.
type agentLaunch struct {
	project *pi.Loaded
	res     *maa.Resource
	resName string
	prefix  string // output prefix, e.g. "[agent] "
	logMode string // --agent-log value
	logName string // log file name used with --agent-log <dir>
	stop    <-chan os.Signal
	// env carries the PI_* variables the protocol requires for agents.
	env []string
}

// startAgents starts and connects every agent declared by the ProjectInterface.
// env carries the PI_* context variables (protocol v2.5.0); the returned agents
// must be stopped by the caller when the run ends. On error no agent is left
// running. The caller has already dropped the agents it does not want, so
// --no-agent is resolved before this point.
func startAgents(project *pi.Loaded, res *maa.Resource, resName string, opt runOptions, stop <-chan os.Signal, env []string) ([]*agentProcess, error) {
	specs := declaredAgents(project.Agent)
	if len(specs) == 0 {
		return nil, nil
	}
	agents := make([]*agentProcess, 0, len(specs))
	for i, spec := range specs {
		launch := agentLaunch{
			project: project,
			res:     res,
			resName: resName,
			prefix:  agentOutputPrefix(i, len(specs)),
			logMode: opt.agentLog,
			logName: agentLogFileName(i, len(specs)),
			stop:    stop,
			env:     env,
		}
		agent, err := launch.start(spec)
		if err != nil {
			stopAgents(agents)
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

// declaredAgents drops agent entries without child_exec so one malformed entry
// cannot fail the whole run.
func declaredAgents(specs pi.Agents) []pi.Agent {
	var declared []pi.Agent
	for _, spec := range specs {
		if strings.TrimSpace(spec.ChildExec) != "" {
			declared = append(declared, spec)
		}
	}
	return declared
}

// agentOutputPrefix labels the output of an agent child process.
func agentOutputPrefix(index, total int) string {
	if total <= 1 {
		return "[agent] "
	}
	return fmt.Sprintf("[agent %d] ", index+1)
}

// agentLogFileName names an agent's log file when --agent-log points at a
// directory.
func agentLogFileName(index, total int) string {
	if total <= 1 {
		return "agent.log"
	}
	return fmt.Sprintf("agent-%d.log", index+1)
}

// start launches the AgentServer child process, connects the AgentClient to it,
// and reports both the successful and the failed outcome.
func (l agentLaunch) start(spec pi.Agent) (*agentProcess, error) {
	label := strings.TrimSpace(strings.Join(append([]string{spec.ChildExec}, spec.ChildArgs...), " "))

	client, err := maa.NewAgentClient(maa.WithIdentifier(spec.Identifier))
	if err != nil {
		return nil, fmt.Errorf("agent %s: create AgentClient: %w", label, err)
	}
	identifier, err := client.Identifier()
	if err != nil || identifier == "" {
		client.Destroy()
		return nil, fmt.Errorf("agent %s: read the AgentClient socket identifier", label)
	}
	if err := client.BindResource(l.res); err != nil {
		client.Destroy()
		return nil, fmt.Errorf("agent %s: bind the AgentClient to resource %s: %w", label, l.resName, err)
	}

	logs, err := openAgentLogs(l.logMode, l.prefix, l.logName)
	if err != nil {
		client.Destroy()
		return nil, fmt.Errorf("agent %s: %w", label, err)
	}

	// Every MaaFramework agent binding reads the socket identifier from its last
	// argument, so it is appended after the declared child_args.
	cmd := exec.Command(execPath(l.project.Dir, spec.ChildExec), append(append([]string{}, spec.ChildArgs...), identifier)...)
	// The PI protocol defines the agent working directory as the directory
	// holding interface.json, which also resolves relative child_exec paths.
	cmd.Dir = l.project.Dir
	// PI_* variables give the agent the client context and the current
	// controller/resource selection (protocol v2.5.0).
	if len(l.env) > 0 {
		cmd.Env = append(os.Environ(), l.env...)
	}
	// Agents run silently in the background: they must not open a console
	// window, and their output goes to maactl or to the configured log file.
	cmd.SysProcAttr = agentSysProcAttr()
	cmd.Stdout, cmd.Stderr = logs.stdout, logs.stderr
	if err := cmd.Start(); err != nil {
		logs.close()
		client.Destroy()
		return nil, fmt.Errorf("agent %s: failed to start: %w", label, err)
	}

	agent := &agentProcess{client: client, cmd: cmd, logs: logs, exited: make(chan struct{})}
	go func() {
		agent.exitErr = cmd.Wait()
		close(agent.exited)
	}()
	fmt.Printf("Agent %s started (pid %d, identifier %s)\n", label, cmd.Process.Pid, identifier)
	if logs.where != "" {
		fmt.Printf("Agent %s log: %s\n", label, logs.where)
	}

	// Connect blocks until the AgentServer connected and registered its custom
	// items, so it runs in its own goroutine and is watched for a child process
	// that exits first, an agent that never connects, and an interrupt.
	fmt.Printf("Connecting agent %s\n", label)
	connected := make(chan error, 1)
	go func() { connected <- client.Connect() }()
	select {
	case err := <-connected:
		if err != nil {
			agent.shutdown()
			return nil, fmt.Errorf("agent %s: failed to connect (identifier %s): %w", label, identifier, err)
		}
	case <-agent.exited:
		exitErr := agent.exitErr
		agent.abandon()
		if exitErr != nil {
			return nil, fmt.Errorf("agent %s: failed to connect: the agent process exited: %v", label, exitErr)
		}
		return nil, fmt.Errorf("agent %s: failed to connect: the agent process exited without connecting", label)
	case <-time.After(agentConnectTimeout):
		agent.abandon()
		return nil, fmt.Errorf("agent %s: failed to connect: no agent server connected within %s (identifier %s)", label, agentConnectTimeout, identifier)
	case <-l.stop:
		agent.abandon()
		return nil, fmt.Errorf("agent %s: interrupted while connecting", label)
	}
	fmt.Printf("Agent %s connected%s\n", label, agent.registrations())
	return agent, nil
}

// registrations describes what the agent server registered, so a successful
// connection also shows what the agent provides.
func (a *agentProcess) registrations() string {
	var parts []string
	if actions, err := a.client.GetCustomActionList(); err == nil && len(actions) > 0 {
		parts = append(parts, "custom actions: "+strings.Join(actions, ", "))
	}
	if recognitions, err := a.client.GetCustomRecognitionList(); err == nil && len(recognitions) > 0 {
		parts = append(parts, "custom recognitions: "+strings.Join(recognitions, ", "))
	}
	if len(parts) == 0 {
		return " (no custom actions or recognitions registered)"
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

// stopAgents shuts down agents in the reverse order they were started.
func stopAgents(agents []*agentProcess) {
	for i := len(agents) - 1; i >= 0; i-- {
		agents[i].shutdown()
	}
}

// shutdown disconnects the client and stops the child process. Disconnecting
// first lets an AgentServer leave its join loop and exit on its own.
func (a *agentProcess) shutdown() {
	if a.client != nil {
		if a.client.Connected() {
			_ = a.client.Disconnect()
		}
		a.client.Destroy()
		a.client = nil
	}
	a.stopProcess()
}

// abandon gives up on an agent whose AgentClient may still be blocked inside
// Connect. Destroying the client releases the connection and its socket, and
// the child process is stopped so nothing is left running. shutdown is not
// called after abandon, so the client is never destroyed twice.
func (a *agentProcess) abandon() {
	if a.client != nil {
		a.client.Destroy()
		a.client = nil
	}
	a.stopProcess()
}

// stopProcess waits briefly for the child process to exit, kills it, and closes
// the log file once no writer can touch it anymore.
func (a *agentProcess) stopProcess() {
	defer a.logs.close()
	if a.cmd == nil || a.cmd.Process == nil {
		return
	}
	select {
	case <-a.exited:
		return
	case <-time.After(agentExitGrace):
	}
	_ = a.cmd.Process.Kill()
	select {
	case <-a.exited:
	case <-time.After(agentExitGrace):
		// The agent ignored the kill; there is nothing else to do on the way out.
	}
}

// agentLogs routes one agent's stdout and stderr.
type agentLogs struct {
	stdout io.Writer
	stderr io.Writer
	closer io.Closer
	where  string // log file path; empty when the output is not written to a file
}

// openAgentLogs resolves --agent-log for one agent: maactl's terminal with an
// [agent] prefix by default, nothing for "off", or one file per agent inside a
// directory. A directory value is created when missing, and each run overwrites
// the previous log file.
func openAgentLogs(mode, prefix, fileName string) (*agentLogs, error) {
	switch {
	case mode == "" || strings.EqualFold(mode, agentLogTerm):
		// The prefix is what tells the user which lines came from the agent
		// instead of from maactl or MaaFramework.
		return &agentLogs{
			stdout: newLinePrefixWriter(os.Stdout, prefix),
			stderr: newLinePrefixWriter(os.Stderr, prefix),
		}, nil
	case strings.EqualFold(mode, agentLogOff):
		// The pipes are still drained, so a chatty agent cannot block on a full
		// pipe while its output is discarded.
		return &agentLogs{stdout: io.Discard, stderr: io.Discard}, nil
	}
	if err := os.MkdirAll(mode, 0o755); err != nil {
		return nil, fmt.Errorf("create agent log directory %s: %w", mode, err)
	}
	file, err := os.Create(filepath.Join(mode, fileName))
	if err != nil {
		return nil, fmt.Errorf("create agent log file: %w", err)
	}
	// Both streams share one file, so MaaFramework merges the child's output into
	// a single pipe and lines cannot interleave.
	return &agentLogs{stdout: file, stderr: file, closer: file, where: file.Name()}, nil
}

// close releases the log file, if any. It is safe to call more than once.
func (l *agentLogs) close() {
	if l == nil || l.closer == nil {
		return
	}
	_ = l.closer.Close()
	l.closer = nil
}

// linePrefixWriter prefixes every line written to it, so agent output stays
// distinguishable from maactl and MaaFramework output.
type linePrefixWriter struct {
	w         io.Writer
	prefix    string
	mu        sync.Mutex
	lineStart bool
}

func newLinePrefixWriter(w io.Writer, prefix string) *linePrefixWriter {
	return &linePrefixWriter{w: w, prefix: prefix, lineStart: true}
}

func (p *linePrefixWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	written := 0
	for written < len(b) {
		if p.lineStart {
			if _, err := io.WriteString(p.w, p.prefix); err != nil {
				return written, err
			}
			p.lineStart = false
		}
		line := b[written:]
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line = line[:i+1]
		}
		if _, err := p.w.Write(line); err != nil {
			return written, err
		}
		written += len(line)
		p.lineStart = line[len(line)-1] == '\n'
	}
	return written, nil
}
