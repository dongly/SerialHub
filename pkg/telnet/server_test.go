// Package telnet provides Telnet server functionality.
package telnet

import (
	"net"
	"testing"
	"time"

	"github.com/yourname/serialhub/internal/testutil"
)

func TestNewTelnetServer(t *testing.T) {
	server, err := NewTelnetServer("127.0.0.1", 2323)
	testutil.AssertNoError(t, err)
	testutil.AssertNotNil(t, server)
	testutil.AssertEqual(t, 2323, server.port)
	testutil.AssertEqual(t, 0, server.ClientCount())
}

func TestNewTelnetServer_InvalidPort(t *testing.T) {
	// Test port out of range
	_, err := NewTelnetServer("127.0.0.1", -1)
	testutil.AssertError(t, err)

	_, err = NewTelnetServer("127.0.0.1", 70000)
	testutil.AssertError(t, err)
}

func TestStartStop(t *testing.T) {
	server, err := NewTelnetServer("127.0.0.1", 2323)
	testutil.AssertNoError(t, err)

	// Start server
	err = server.Start()
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, server.IsRunning())

	// Cannot start again
	err = server.Start()
	testutil.AssertError(t, err)

	// Stop server
	err = server.Stop()
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, false, server.IsRunning())

	// Cannot stop again
	err = server.Stop()
	testutil.AssertError(t, err)
}

func TestStart_InvalidPort(t *testing.T) {
	server, err := NewTelnetServer("127.0.0.1", 2323)
	testutil.AssertNoError(t, err)

	// Try to start on a port that's already in use
	err = server.Start()
	testutil.AssertNoError(t, err)

	// Try to start another server on the same port (this will fail)
	server2, err := NewTelnetServer("127.0.0.1", 2323)
	testutil.AssertNoError(t, err)

	err = server2.Start()
	testutil.AssertError(t, err)

	// Cleanup
	server.Stop()
}

func TestAcceptClient(t *testing.T) {
	server, err := NewTelnetServer("127.0.0.1", 0)
	testutil.AssertNoError(t, err)

	err = server.Start()
	testutil.AssertNoError(t, err)

	// Get the actual port
	addr := server.listener.Addr().String()
	host, port, _ := net.SplitHostPort(addr)

	// Connect a client
	conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	testutil.AssertNoError(t, err)
	defer conn.Close()

	// Wait for client to be accepted
	time.Sleep(100 * time.Millisecond)

	// Check client count
	testutil.AssertEqual(t, 1, server.ClientCount())

	// Check welcome message
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, err := conn.Read(buf)
	testutil.AssertNoError(t, err)
	testutil.AssertContains(t, string(buf[:n]), "Connected to SerialHub")

	// Cleanup
	server.Stop()
}

func TestDisconnectClient(t *testing.T) {
	server, err := NewTelnetServer("127.0.0.1", 0)
	testutil.AssertNoError(t, err)

	err = server.Start()
	testutil.AssertNoError(t, err)

	// Connect a client
	addr := server.listener.Addr().String()
	host, port, _ := net.SplitHostPort(addr)

	conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	testutil.AssertNoError(t, err)
	defer conn.Close()

	// Wait for client to be accepted
	time.Sleep(100 * time.Millisecond)

	// Get client ID
	clients := server.GetClients()
	testutil.AssertEqual(t, 1, len(clients))
	clientID := clients[0]

	// Disconnect client
	err = server.DisconnectClient(clientID)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, 0, server.ClientCount())

	// Try to disconnect again (should fail)
	err = server.DisconnectClient(clientID)
	testutil.AssertError(t, err)

	// Cleanup
	server.Stop()
}

func TestBroadcast_Multiple(t *testing.T) {
	server, err := NewTelnetServer("127.0.0.1", 0)
	testutil.AssertNoError(t, err)

	err = server.Start()
	testutil.AssertNoError(t, err)

	// Connect multiple clients
	addr := server.listener.Addr().String()
	host, port, _ := net.SplitHostPort(addr)

	clients := make([]net.Conn, 3)
	for i := 0; i < 3; i++ {
		conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
		testutil.AssertNoError(t, err)
		clients[i] = conn

		// Read welcome message
		buf := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		_, err = conn.Read(buf)
		testutil.AssertNoError(t, err)
	}

	// Wait for all clients to be accepted
	time.Sleep(100 * time.Millisecond)

	testutil.AssertEqual(t, 3, server.ClientCount())

	// Broadcast data
	testData := []byte("Hello from server!\r\n")
	count := server.Broadcast(testData)
	testutil.AssertEqual(t, 3, count)

	// Verify all clients received the data
	for _, conn := range clients {
		buf := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, err := conn.Read(buf)
		testutil.AssertNoError(t, err)
		testutil.AssertBytesEqual(t, testData, buf[:n])
	}

	// Cleanup
	for _, conn := range clients {
		conn.Close()
	}
	server.Stop()
}

func TestSendToClient(t *testing.T) {
	server, err := NewTelnetServer("127.0.0.1", 0)
	testutil.AssertNoError(t, err)

	err = server.Start()
	testutil.AssertNoError(t, err)

	// Connect a client
	addr := server.listener.Addr().String()
	host, port, _ := net.SplitHostPort(addr)

	conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	testutil.AssertNoError(t, err)
	defer conn.Close()

	// Read welcome message
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, err = conn.Read(buf)
	testutil.AssertNoError(t, err)

	// Wait for client to be accepted
	time.Sleep(100 * time.Millisecond)

	// Get client ID
	clients := server.GetClients()
	testutil.AssertEqual(t, 1, len(clients))
	clientID := clients[0]

	// Send data to specific client
	testData := []byte("Hello specific client!\r\n")
	err = server.SendToClient(clientID, testData)
	testutil.AssertNoError(t, err)

	// Verify client received the data
	buf = make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, err := conn.Read(buf)
	testutil.AssertNoError(t, err)
	testutil.AssertBytesEqual(t, testData, buf[:n])

	// Try to send to non-existent client
	err = server.SendToClient("non-existent-id", testData)
	testutil.AssertError(t, err)

	// Cleanup
	server.Stop()
}

func TestDataChan(t *testing.T) {
	server, err := NewTelnetServer("127.0.0.1", 0)
	testutil.AssertNoError(t, err)

	err = server.Start()
	testutil.AssertNoError(t, err)

	// Connect a client
	addr := server.listener.Addr().String()
	host, port, _ := net.SplitHostPort(addr)

	conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	testutil.AssertNoError(t, err)
	defer conn.Close()

	// Read welcome message
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, err = conn.Read(buf)
	testutil.AssertNoError(t, err)

	// Wait for client to be accepted
	time.Sleep(100 * time.Millisecond)

	// Send data from client
	testData := []byte("Hello from client!\r\n")
	_, err = conn.Write(testData)
	testutil.AssertNoError(t, err)

	// Receive data from data channel
	data := testutil.WaitForChannel(t, server.DataChan(), 2*time.Second)
	testutil.AssertBytesEqual(t, testData, data)

	// Cleanup
	server.Stop()
}

func TestFullWorkflow(t *testing.T) {
	server, err := NewTelnetServer("127.0.0.1", 0)
	testutil.AssertNoError(t, err)

	// Start server
	err = server.Start()
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, server.IsRunning())

	// Connect client
	addr := server.listener.Addr().String()
	host, port, _ := net.SplitHostPort(addr)

	conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	testutil.AssertNoError(t, err)
	defer conn.Close()

	// Read welcome message
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, err := conn.Read(buf)
	testutil.AssertNoError(t, err)
	testutil.AssertContains(t, string(buf[:n]), "Connected to SerialHub")

	// Wait for client to be accepted
	time.Sleep(100 * time.Millisecond)

	// Get client info
	clients := server.GetClients()
	testutil.AssertEqual(t, 1, len(clients))
	clientID := clients[0]
	testutil.AssertNotNil(t, clientID)

	// Send data from client to server
	testData := []byte("Test command\r\n")
	_, err = conn.Write(testData)
	testutil.AssertNoError(t, err)

	// Receive data from data channel
	data := testutil.WaitForChannel(t, server.DataChan(), 2*time.Second)
	testutil.AssertBytesEqual(t, testData, data)

	// Broadcast data from server to client
	broadcastData := []byte("Server response\r\n")
	count := server.Broadcast(broadcastData)
	testutil.AssertEqual(t, 1, count)

	// Verify client received broadcast
	buf = make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, err = conn.Read(buf)
	testutil.AssertNoError(t, err)
	testutil.AssertBytesEqual(t, broadcastData, buf[:n])

	// Send to specific client
	specificData := []byte("Specific message\r\n")
	err = server.SendToClient(clientID, specificData)
	testutil.AssertNoError(t, err)

	// Verify client received specific message
	buf = make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, err = conn.Read(buf)
	testutil.AssertNoError(t, err)
	testutil.AssertBytesEqual(t, specificData, buf[:n])

	// Disconnect client
	err = server.DisconnectClient(clientID)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, 0, server.ClientCount())

	// Stop server
	err = server.Stop()
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, false, server.IsRunning())
}
