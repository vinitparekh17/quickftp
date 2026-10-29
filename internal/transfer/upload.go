package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"time"
)

// uploadStaged only repeats a transient failure after inspecting this run's
// private staging name. Never replay a promotion or overwrite a live target.
func (e *Engine) uploadStaged(ctx context.Context, id int64, f prepared, target, staging string) error {
	const attempts = 3
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt > 1 {
			digest, size, err := hashFile(f.LocalPath)
			if err != nil {
				return fmt.Errorf("recheck local artifact before retry: %w", err)
			}
			if digest != f.digest || size != f.size {
				return errors.New("local artifact changed during deployment; refusing to retry")
			}
		}
		src, err := os.Open(f.LocalPath)
		if err != nil {
			return err
		}
		uploadErr := e.withGate(ctx, func() error {
			e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Running", "")
			return e.Client.Upload(ctx, staging, &engineProgressReader{
				r:       src,
				onBytes: func(n int64) { e.fileTransfer(id, target, f.LocalPath, f.size, n, "Running", "") },
			})
		})
		closeErr := src.Close()
		if err := ctx.Err(); err != nil {
			return err
		}
		if closeErr != nil {
			return fmt.Errorf("close local artifact: %w", closeErr)
		}
		matches, verifyErr := e.remoteMatches(ctx, staging, f.digest)
		if matches && verifyErr == nil {
			if uploadErr != nil {
				e.log("stdout", fmt.Sprintf("Recovered upload %s: staged SHA-256 verified despite server error: %v", f.relative, uploadErr))
			}
			return nil
		}
		if uploadErr == nil {
			if verifyErr != nil {
				return fmt.Errorf("verify staged upload %s: %w", f.relative, verifyErr)
			}
			return fmt.Errorf("verify staged upload %s: SHA-256 mismatch", f.relative)
		}
		// An unreadable staging file has an unknown outcome. Only confirmed missing
		// or readable-but-mismatched content permits another STOR to this private name.
		if verifyErr != nil && !errors.Is(verifyErr, ErrNotFound) {
			return fmt.Errorf("upload %s outcome unresolved: %w; staging verification failed: %w", f.relative, uploadErr, verifyErr)
		}
		state := "SHA-256 mismatch (incomplete or different staged content)"
		if errors.Is(verifyErr, ErrNotFound) {
			state = "staging file not found"
		}
		if attempt == attempts || !transientUploadError(uploadErr) {
			return fmt.Errorf("upload %s failed after %d attempt(s): %w; %s; existing target was not promoted or replaced for this file", f.relative, attempt, uploadErr, state)
		}
		e.log("stderr", fmt.Sprintf("Upload %s interrupted: %v; %s. Retrying verified staging operation (%d/%d)", f.relative, uploadErr, state, attempt+1, attempts))
		if err := sleepWithContext(ctx, time.Duration(attempt)*500*time.Millisecond); err != nil {
			return err
		}
	}
	return errors.New("upload retry limit reached")
}

func transientUploadError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var reply *textproto.Error
	if errors.As(err, &reply) {
		switch reply.Code {
		case 421, 425, 426:
			return true
		default:
			return false
		}
	}
	var network net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) || errors.As(err, &network)
}
