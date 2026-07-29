package security

import "testing"

func TestSingleton(t *testing.T) {
	d := t.TempDir()
	a, e := Acquire(d)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Release()
	if _, e = Acquire(d); e == nil {
		t.Fatal("second lock")
	}
}
