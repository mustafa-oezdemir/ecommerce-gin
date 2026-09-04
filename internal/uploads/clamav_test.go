package uploads

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestClamAVScannerStreamsContent(t *testing.T) {
	address, received, stop := fakeClamAV(t, "stream: OK\x00")
	defer stop()
	scanner, err := NewClamAVScanner(address, time.Second)
	if err != nil {
		t.Fatalf("create scanner: %v", err)
	}
	payload := []byte("safe-image-content")
	if err := scanner.Scan(context.Background(), payload); err != nil {
		t.Fatalf("scan content: %v", err)
	}
	if got := <-received; string(got) != string(payload) {
		t.Fatalf("scanner sent %q, expected %q", got, payload)
	}
}

func TestClamAVScannerRejectsThreat(t *testing.T) {
	address, _, stop := fakeClamAV(t, "stream: Eicar-Signature FOUND\x00")
	defer stop()
	scanner, _ := NewClamAVScanner(address, time.Second)
	if err := scanner.Scan(context.Background(), []byte("threat")); !errors.Is(err, ErrThreatDetected) {
		t.Fatalf("expected threat detection, got %v", err)
	}
}

func TestClamAVScannerFailsClosedOnUnexpectedResponse(t *testing.T) {
	address, _, stop := fakeClamAV(t, "stream: UNKNOWN COMMAND\x00")
	defer stop()
	scanner, _ := NewClamAVScanner(address, time.Second)
	if err := scanner.Scan(context.Background(), []byte("content")); !errors.Is(err, ErrScannerUnavailable) {
		t.Fatalf("expected unavailable scanner error, got %v", err)
	}
}

func fakeClamAV(t *testing.T, response string) (string, <-chan []byte, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	received := make(chan []byte, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		command := make([]byte, len("zINSTREAM\x00"))
		if _, err := io.ReadFull(connection, command); err != nil || string(command) != "zINSTREAM\x00" {
			return
		}
		var payload []byte
		for {
			var sizeBuffer [4]byte
			if _, err := io.ReadFull(connection, sizeBuffer[:]); err != nil {
				return
			}
			size := binary.BigEndian.Uint32(sizeBuffer[:])
			if size == 0 {
				break
			}
			chunk := make([]byte, size)
			if _, err := io.ReadFull(connection, chunk); err != nil {
				return
			}
			payload = append(payload, chunk...)
		}
		received <- payload
		_, _ = io.WriteString(connection, response)
	}()
	return listener.Addr().String(), received, func() {
		_ = listener.Close()
		<-done
	}
}
