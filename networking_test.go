package veneur

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"io/ioutil"
	"os"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/protocol"
)

func TestMultipleListeners(t *testing.T) {
	srv := &Server{}
	srv.shutdown = make(chan struct{})

	dir, err := ioutil.TempDir("", "unix-listener")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	addrNet, err := protocol.ResolveAddr(fmt.Sprintf("unix://%s/socket", dir))
	require.NoError(t, err)
	addr, ok := addrNet.(*net.UnixAddr)
	require.True(t, ok)

	done, _ := startSSFUnix(srv, addr)
	assert.Panics(t, func() {
		srv2 := &Server{}
		startSSFUnix(srv2, addr)
	})
	close(srv.shutdown)

	// Wait for the server to actually shut down:
	<-done

	srv3 := &Server{}
	srv3.shutdown = make(chan struct{})
	startSSFUnix(srv3, addr)
	close(srv3.shutdown)
}

func TestConnectUNIX(t *testing.T) {
	srv := &Server{}
	srv.shutdown = make(chan struct{})

	dir, err := ioutil.TempDir("", "unix-listener")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	addrNet, err := protocol.ResolveAddr(fmt.Sprintf("unix://%s/socket", dir))
	require.NoError(t, err)
	addr, ok := addrNet.(*net.UnixAddr)
	require.True(t, ok)
	startSSFUnix(srv, addr)

	conns := make(chan struct{})
	for i := 0; i < 5; i++ {
		n := i
		go func() {
			// Dial the server, send it invalid data, wait
			// for it to hang up:
			c, err := net.DialUnix("unix", nil, addr)
			assert.NoError(t, err, "Connecting %d", n)
			wrote, err := c.Write([]byte("foo"))
			if !assert.NoError(t, err, "Writing to %d", n) {
				return
			}
			assert.Equal(t, 3, wrote, "Writing to %d", n)
			assert.NotNil(t, c)

			n, _ = c.Read(make([]byte, 20))
			assert.Equal(t, 0, n)

			err = c.Close()
			assert.NoError(t, err)

			conns <- struct{}{}
		}()
	}
	timeout := time.After(3 * time.Second)
	for i := 0; i < 5; i++ {
		select {
		case <-timeout:
			t.Fatalf("Timed out waiting for connection, %d made it", i)
		case <-conns:
			// pass
		}
	}
	close(srv.shutdown)
}

func TestConnectUNIXStatsd(t *testing.T) {
	srv := &Server{}
	srv.shutdown = make(chan struct{})
	srv.metricMaxLength = 4096

	dir, err := ioutil.TempDir("", "unix-domain-listener")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	addrNet, err := protocol.ResolveAddr(fmt.Sprintf("unixgram://%s/datagramsocket", dir))
	require.NoError(t, err)
	addr, ok := addrNet.(*net.UnixAddr)
	require.True(t, ok)
	statsdPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, 4097)
		},
	}
	startStatsdUnix(srv, addr, statsdPool)

	conns := make(chan struct{})
	for i := 0; i < 5; i++ {
		n := i
		go func() {
			// Dial the server, send it invalid data, wait
			// for it to hang up:
			c, err := net.DialUnix("unixgram", nil, addr)
			assert.NoError(t, err, "Connecting %d", n)
			wrote, err := c.Write([]byte("foo"))
			if !assert.NoError(t, err, "Writing to %d", n) {
				fmt.Println("Error writing to socket")
				return
			}
			assert.Equal(t, 3, wrote, "Writing to %d", n)
			assert.NotNil(t, c)
			c.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, _ = c.Read(make([]byte, 20))
			assert.Equal(t, 0, n)

			err = c.Close()
			assert.NoError(t, err)
			conns <- struct{}{}
		}()
	}
	timeout := time.After(3 * time.Second)
	for i := 0; i < 5; i++ {
		select {
		case <-timeout:
			t.Fatalf("Timed out waiting for connection, %d made it", i)
		case <-conns:
			// pass
		}
	}
	close(srv.shutdown)
}

func TestConnectTCPSSF(t *testing.T) {
	srv := &Server{}
	srv.shutdown = make(chan struct{})

	addrNet, err := protocol.ResolveAddr("tcp://127.0.0.1:8125")
	require.NoError(t, err)
	addr, ok := addrNet.(*net.TCPAddr)
	require.True(t, ok)
	startSSFTCP(srv, addr, &sync.Pool{})

	conns := make(chan struct{})
	for i := 0; i < 5; i++ {
		n := i
		go func() {
			c, err := net.Dial("tcp", addr.String())
			assert.NoError(t, err, "Connecting %d", n)
			wrote, err := c.Write([]byte("foo"))
			if !assert.NoError(t, err, "Writing to %d", n) {
				return
			}
			assert.Equal(t, 3, wrote, "Writing to %d", n)
			assert.NotNil(t, c)

			err = c.Close()
			assert.NoError(t, err)

			conns <- struct{}{}
		}()
	}
	timeout := time.After(3 * time.Second)
	for i := 0; i < 5; i++ {
		select {
		case <-timeout:
			t.Fatalf("Timed out waiting for connection, %d made it", i)
		case <-conns:
			// pass
		}
	}
	close(srv.shutdown)
}
