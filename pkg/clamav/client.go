package clamav

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

const (
	// DefaultChunkSize is the default size for streaming chunks to ClamAV
	DefaultChunkSize = 2048
	// StatusClean indicates the file is clean
	StatusClean = "clean"
	// StatusInfected indicates the file contains malware
	StatusInfected = "infected"
	// StatusError indicates an error occurred during scanning
	StatusError = "error"
)

var (
	// ErrConnectionFailed is returned when connection to ClamAV fails
	ErrConnectionFailed = errors.New("clamav: connection failed")
	// ErrScanFailed is returned when the scan operation fails
	ErrScanFailed = errors.New("clamav: scan failed")
	// ErrTimeout is returned when the scan times out
	ErrTimeout = errors.New("clamav: timeout")
)

// ScanResult contains the result of a ClamAV scan
type ScanResult struct {
	Status    string `json:"status"`     // clean, infected, error
	VirusName string `json:"virus_name"` // name of detected virus (if infected)
	Error     string `json:"error"`      // error message (if error)
}

// Client is a ClamAV client for scanning files
type Client struct {
	address string
	timeout time.Duration
}

// NewClient creates a new ClamAV client
func NewClient(address string, timeout time.Duration) *Client {
	return &Client{
		address: address,
		timeout: timeout,
	}
}

// isTimeoutError checks if the error is a network timeout
func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// wrapError wraps an error with the appropriate sentinel error (ErrTimeout or provided fallback)
func wrapError(err error, fallback error) error {
	if isTimeoutError(err) {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	return fmt.Errorf("%w: %v", fallback, err)
}

// Ping checks if the ClamAV daemon is responsive
func (c *Client) Ping(ctx context.Context) error {
	dialer := &net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return wrapError(err, ErrConnectionFailed)
	}
	defer conn.Close()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.timeout)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return wrapError(err, ErrConnectionFailed)
	}

	_, err = conn.Write([]byte("nPING\n"))
	if err != nil {
		return wrapError(err, ErrConnectionFailed)
	}

	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		return wrapError(err, ErrConnectionFailed)
	}

	response = strings.TrimSpace(response)
	if response != "PONG" {
		return fmt.Errorf("%w: unexpected PING response: %s", ErrConnectionFailed, response)
	}

	return nil
}

// Scan scans content from a reader using the INSTREAM protocol
func (c *Client) Scan(ctx context.Context, r io.Reader) (*ScanResult, error) {
	dialer := &net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return errorResult("connection failed", err), wrapError(err, ErrConnectionFailed)
	}
	defer conn.Close()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.timeout)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return errorResult("failed to set deadline", err), wrapError(err, ErrConnectionFailed)
	}

	// Send INSTREAM command
	if _, err = conn.Write([]byte("nINSTREAM\n")); err != nil {
		return errorResult("failed to send INSTREAM command", err), wrapError(err, ErrScanFailed)
	}

	// Stream file content using chunked writer
	if err := c.streamContent(conn, r); err != nil {
		return errorResult("failed to stream content", err), wrapError(err, ErrScanFailed)
	}

	// Send terminator (4 zero bytes)
	if _, err := conn.Write([]byte{0, 0, 0, 0}); err != nil {
		return errorResult("failed to send terminator", err), wrapError(err, ErrScanFailed)
	}

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return errorResult("failed to read response", err), wrapError(err, ErrScanFailed)
	}

	return parseResponse(strings.TrimSpace(response)), nil
}

// streamContent streams the content to ClamAV using the INSTREAM chunked protocol
func (c *Client) streamContent(conn net.Conn, r io.Reader) error {
	// The ClamAV INSTREAM protocol format is:
	// [4-byte length][data chunk][4-byte length][data chunk]...[0x00000000]
	buf := make([]byte, DefaultChunkSize)
	lenBuf := make([]byte, 4)

	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			// Send chunk length (4-byte big-endian)
			binary.BigEndian.PutUint32(lenBuf, uint32(n))
			if _, err := conn.Write(lenBuf); err != nil {
				return fmt.Errorf("failed to send chunk length: %w", err)
			}

			// Send chunk data
			if _, err := conn.Write(buf[:n]); err != nil {
				return fmt.Errorf("failed to send chunk data: %w", err)
			}
		}

		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("failed to read content: %w", readErr)
		}
	}
}

// errorResult creates an error ScanResult with the given message and error
func errorResult(msg string, err error) *ScanResult {
	errMsg := msg
	if err != nil {
		errMsg = fmt.Sprintf("%s: %v", msg, err)
	}
	return &ScanResult{
		Status: StatusError,
		Error:  errMsg,
	}
}

// parseResponse parses the ClamAV response string
// Format: "stream: OK" for clean, "stream: <virus name> FOUND" for infected
func parseResponse(response string) *ScanResult {
	// Remove "stream: " prefix if present
	response = strings.TrimPrefix(response, "stream: ")

	if response == "OK" {
		return &ScanResult{
			Status: StatusClean,
		}
	}

	if strings.HasSuffix(response, " FOUND") {
		virusName := strings.TrimSuffix(response, " FOUND")
		return &ScanResult{
			Status:    StatusInfected,
			VirusName: virusName,
		}
	}

	// Check for error responses
	if strings.HasPrefix(response, "ERROR") || strings.Contains(response, "error") {
		return &ScanResult{
			Status: StatusError,
			Error:  response,
		}
	}

	// Unknown response
	return &ScanResult{
		Status: StatusError,
		Error:  fmt.Sprintf("unknown response: %s", response),
	}
}
