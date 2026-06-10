package command

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShutdownTailscaleProxyUsesDedicatedTimeout(t *testing.T) {
	deadlineSeen := false

	err := shutdownTailscaleProxy(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		assert.WithinDuration(t, time.Now().Add(tailscaleServeShutdownTimeout), deadline, time.Second)
		assert.NoError(t, ctx.Err())
		deadlineSeen = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, deadlineSeen)
}
