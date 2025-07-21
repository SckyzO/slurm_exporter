package collector

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

var (
	commandTimeout time.Duration
)

// SetCommandTimeout sets the timeout for external commands.
func SetCommandTimeout(t time.Duration) {
	commandTimeout = t
}

// Execute is a wrapper around exec.CommandContext to provide logging and a timeout.
var Execute = func(logger log.Logger, command string, args []string) ([]byte, error) {
	_ = level.Debug(logger).Log("msg", "Executing command", "command", command, "args", strings.Join(args, " "))

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the error is due to the context deadline exceeding.
		if ctx.Err() == context.DeadlineExceeded {
			_ = level.Error(logger).Log("msg", "Command timed out", "command", command, "args", strings.Join(args, " "), "timeout", commandTimeout)
			return nil, ctx.Err()
		}
		_ = level.Error(logger).Log("msg", "Failed to execute command", "command", command, "args", strings.Join(args, " "), "output", string(out), "err", err)
		return nil, err
	}

	_ = level.Debug(logger).Log("msg", "Command executed successfully", "command", command)
	return out, nil
}
