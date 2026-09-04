package uploads

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

var (
	ErrThreatDetected     = errors.New("malware detected")
	ErrScannerUnavailable = errors.New("malware scanner unavailable")
)

type Scanner interface {
	Scan(context.Context, []byte) error
}

type ClamAVScanner struct {
	address string
	timeout time.Duration
}

func NewClamAVScanner(address string, timeout time.Duration) (*ClamAVScanner, error) {
	if strings.TrimSpace(address) == "" {
		return nil, errors.New("uploads: ClamAV address is required")
	}
	if timeout <= 0 {
		return nil, errors.New("uploads: ClamAV timeout must be positive")
	}
	return &ClamAVScanner{address: address, timeout: timeout}, nil
}

func (scanner *ClamAVScanner) Scan(ctx context.Context, data []byte) error {
	scanContext, cancel := context.WithTimeout(ctx, scanner.timeout)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(scanContext, "tcp", scanner.address)
	if err != nil {
		return fmt.Errorf("%w: connect: %v", ErrScannerUnavailable, err)
	}
	defer connection.Close()
	if deadline, ok := scanContext.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}

	if err := writeAll(connection, []byte("zINSTREAM\x00")); err != nil {
		return fmt.Errorf("%w: send command: %v", ErrScannerUnavailable, err)
	}
	for offset := 0; offset < len(data); {
		end := min(offset+32*1024, len(data))
		var chunkSize [4]byte
		binary.BigEndian.PutUint32(chunkSize[:], uint32(end-offset))
		if err := writeAll(connection, chunkSize[:]); err != nil {
			return fmt.Errorf("%w: send chunk size: %v", ErrScannerUnavailable, err)
		}
		if err := writeAll(connection, data[offset:end]); err != nil {
			return fmt.Errorf("%w: send chunk: %v", ErrScannerUnavailable, err)
		}
		offset = end
	}
	if err := writeAll(connection, []byte{0, 0, 0, 0}); err != nil {
		return fmt.Errorf("%w: finish stream: %v", ErrScannerUnavailable, err)
	}

	response, err := bufio.NewReader(connection).ReadString(0)
	if err != nil {
		return fmt.Errorf("%w: read response: %v", ErrScannerUnavailable, err)
	}
	response = strings.TrimSpace(strings.TrimSuffix(response, "\x00"))
	if strings.HasSuffix(response, " OK") {
		return nil
	}
	if strings.Contains(response, " FOUND") {
		return ErrThreatDetected
	}
	return fmt.Errorf("%w: unexpected response", ErrScannerUnavailable)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrUnexpectedEOF
		}
		data = data[written:]
	}
	return nil
}
