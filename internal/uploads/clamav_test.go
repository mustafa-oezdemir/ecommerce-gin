package uploads

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestCheckedUint32RejectsOverflow(t *testing.T) {
	value, err := checkedUint32(32 * 1024)
	if err != nil || value != 32*1024 {
		t.Fatalf("expected valid ClamAV chunk size, value=%d err=%v", value, err)
	}
	if _, err := checkedUint32(-1); err == nil {
		t.Fatal("expected negative value to fail")
	}
	if strconv.IntSize == 64 {
		if _, err := checkedUint32(int64ToInt(t, int64(math.MaxUint32)+1)); err == nil {
			t.Fatal("expected uint32 overflow to fail")
		}
	}
}

func int64ToInt(t *testing.T, value int64) int {
	t.Helper()
	converted := int(value)
	if int64(converted) != value {
		t.Fatalf("test value %d does not fit in int", value)
	}
	return converted
}

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
