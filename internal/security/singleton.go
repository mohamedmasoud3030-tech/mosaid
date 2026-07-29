package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

type Lock struct{ dir string }

func Acquire(data string) (*Lock, error) {
	d := filepath.Join(data, "runtime.lock")
	if err := os.MkdirAll(data, 0700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(d, 0700); err == nil {
		if e := os.WriteFile(filepath.Join(d, "pid"), []byte(strconv.Itoa(os.Getpid())), 0600); e != nil {
			os.RemoveAll(d)
			return nil, e
		}
		return &Lock{d}, nil
	}
	b, _ := os.ReadFile(filepath.Join(d, "pid"))
	pid, _ := strconv.Atoi(string(b))
	if pid > 0 && syscall.Kill(pid, 0) == nil {
		return nil, fmt.Errorf("mosaid already running pid=%d", pid)
	}
	if err := os.RemoveAll(d); err != nil {
		return nil, err
	}
	if err := os.Mkdir(d, 0700); err != nil {
		return nil, errors.New("singleton lock unavailable")
	}
	_ = os.WriteFile(filepath.Join(d, "pid"), []byte(strconv.Itoa(os.Getpid())), 0600)
	return &Lock{d}, nil
}
func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	return os.RemoveAll(l.dir)
}
