// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
)

const applyLockTimeout = 2 * time.Minute

func runLockedApply(runDirectory string, operation func() (coreapply.Result, error)) error {
	lock, err := acquireApplyLock(runDirectory)
	if err != nil {
		return err
	}
	defer lock.Close()
	result, err := runApplyOperation(runDirectory, operation)
	writeJSON(result)
	return err
}

func runLockedApplyResult(runDirectory string, operation func() (coreapply.Result, error)) (coreapply.Result, error) {
	lock, err := acquireApplyLock(runDirectory)
	if err != nil {
		return coreapply.Result{}, err
	}
	defer lock.Close()
	return runApplyOperation(runDirectory, operation)
}

func withOperationLock(runDirectory string, operation func() error) error {
	lock, err := acquireApplyLock(runDirectory)
	if err != nil {
		return err
	}
	defer lock.Close()
	return operation()
}

func runApplyOperation(runDirectory string, operation func() (coreapply.Result, error)) (coreapply.Result, error) {
	result, err := operation()
	return recordApplyResult(runDirectory, result, err)
}

func recordApplyResult(runDirectory string, result coreapply.Result, err error) (coreapply.Result, error) {
	if err != nil {
		result.OK = false
		result.Error = err.Error()
	}
	now := time.Now().UTC()
	if recordErr := writeApplyRecord(runDirectory, coreapply.Record{
		Sequence: strconv.FormatInt(now.UnixNano(), 10), Timestamp: now.Format(time.RFC3339Nano), Result: result,
	}); recordErr != nil {
		if err != nil {
			err = errors.Join(err, recordErr)
		} else {
			err = recordErr
		}
		result.OK = false
		result.Error = err.Error()
		return result, err
	}
	return result, err
}

func acquireApplyLock(runDirectory string) (*os.File, error) {
	ctx, cancel := context.WithTimeout(context.Background(), applyLockTimeout)
	defer cancel()
	return acquireApplyLockContext(ctx, runDirectory)
}

func acquireApplyLockContext(ctx context.Context, runDirectory string) (*os.File, error) {
	if err := os.MkdirAll(runDirectory, 0o750); err != nil {
		return nil, fmt.Errorf("create runtime directory for Apply lock: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(runDirectory, "operation.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Apply lock: %w", err)
	}
	for {
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			lock.Close()
			return nil, fmt.Errorf("lock Apply transaction: %w", err)
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			lock.Close()
			return nil, fmt.Errorf("lock Apply transaction: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func writeApplyRecord(runDirectory string, record coreapply.Record) error {
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return linuxAtomicWrite(filepath.Join(runDirectory, "last-apply.json"), append(encoded, '\n'))
}

func linuxAtomicWrite(path string, content []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".steer.result.*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}
