package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseListeningPorts(t *testing.T) {
	output := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0B00007F:B0C9 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 3659499 2 00000000672f0a7f 100 0 0 10 0
   1: 00000000:18EB 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 3661880 1 0000000043b2e96e 100 0 0 10 0
   2: 00000000:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 3660278 1 00000000d2afcd77 100 0 0 10 0
   3: 0100007F:1F90 00000000:0000 01 00000000:00000000 00:00000000 00000000     0        0 0 0 0 0 0
`

	ports, err := parseListeningPorts(output)
	require.NoError(t, err)

	assert.Equal(t, []int{0xB0C9, 0x18EB, 0x0BB8}, ports)
}

func TestParseListeningPorts_IPv6(t *testing.T) {
	output := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 00000000000000000000000000000000 100 0 0 10 0
`

	ports, err := parseListeningPorts(output)
	require.NoError(t, err)

	assert.Equal(t, []int{0x1F90}, ports)
}

func TestParseListeningPorts_DualStackListenerIsListedOnce(t *testing.T) {
	output := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
    0: 00000000:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000      0        0 3660278 1 00000000d2afcd77 100 0 0 10 0
    1: 00000000000000000000000000000000:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000      0        0 12345 1 00000000000000000000000000000000 100 0 0 10 0
`

	ports, err := parseListeningPorts(output)
	require.NoError(t, err)

	assert.Equal(t, []int{0x0BB8}, ports, "the same port on IPv4 and IPv6 is one listener")
}

func TestParseProxyTarget(t *testing.T) {
	target := parseProxyTarget("http://172.18.0.4:80")
	assert.Equal(t, "172.18.0.4", target.host)
	assert.Equal(t, "80", target.port)

	target = parseProxyTarget("http://172.18.0.4")
	assert.Equal(t, "172.18.0.4", target.host)
	assert.Equal(t, "80", target.port)

	target = parseProxyTarget("https://172.18.0.4")
	assert.Equal(t, "443", target.port)
}

func TestTailscaleServeTargetFor(t *testing.T) {
	target, ok := tailscaleServeTargetFor(healthyServeStatus(), "write", 443)
	require.True(t, ok)
	assert.Equal(t, "172.18.0.4", target.host)
	assert.Equal(t, "80", target.port)

	_, ok = tailscaleServeTargetFor(healthyServeStatus(), "write", 8443)
	assert.False(t, ok, "a Web entry on a different service port doesn't match")

	webAndTCPStatus := []byte(`{"Services":{"svc:write":{"Web":{"write.lemur-carat.ts.net:443":{"Handlers":{"/":{"Proxy":"http://172.18.0.4:80"}}}},"TCP":{"8443":{"TCPForward":"172.18.0.4:5432"}}}}}`)
	target, ok = tailscaleServeTargetFor(webAndTCPStatus, "write", 8443)
	require.True(t, ok, "a mismatched Web entry doesn't hide a matching TCP entry")
	assert.Equal(t, "tcp", target.scheme)
	assert.Equal(t, "172.18.0.4", target.host)
	assert.Equal(t, "5432", target.port)

	_, ok = tailscaleServeTargetFor(healthyServeStatus(), "budget", 443)
	assert.False(t, ok)

	_, ok = tailscaleServeTargetFor([]byte("not json"), "write", 443)
	assert.False(t, ok)

	tcpStatus := []byte(`{"Services":{"svc:postgres":{"TCP":{"5432":{"TCPForward":"172.18.0.5:5432"}}}}}`)
	target, ok = tailscaleServeTargetFor(tcpStatus, "postgres", 5432)
	require.True(t, ok)
	assert.Equal(t, "172.18.0.5", target.host)
	assert.Equal(t, "5432", target.port)

	_, ok = tailscaleServeTargetFor(tcpStatus, "postgres", 5433)
	assert.False(t, ok, "wrong service port doesn't match")
}
