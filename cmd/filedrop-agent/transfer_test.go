package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTransferTimeout(t *testing.T) {
	cases := []struct {
		size int64
		want time.Duration
	}{
		{0, 60 * time.Second},
		{-5, 60 * time.Second},
		{20 << 20, 70 * time.Second},
		{10 << 30, 20 * time.Minute},
	}
	for _, c := range cases {
		if got := transferTimeout(c.size); got != c.want {
			t.Errorf("transferTimeout(%d) = %s, want %s", c.size, got, c.want)
		}
	}
}

func TestChmodDirsWorld(t *testing.T) {
	root := filepath.Join(t.TempDir(), "top")
	os.MkdirAll(filepath.Join(root, "a", "b"), 0700)
	f := filepath.Join(root, "a", "f.txt")
	os.WriteFile(f, []byte("x"), 0600)

	if err := chmodDirsWorld(root); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{root, filepath.Join(root, "a"), filepath.Join(root, "a", "b")} {
		st, _ := os.Stat(d)
		if st.Mode().Perm() != 0777 {
			t.Errorf("%s: %v", d, st.Mode().Perm())
		}
	}
	st, _ := os.Stat(f)
	if st.Mode().Perm() != 0600 {
		t.Errorf("file mode changed: %v", st.Mode().Perm())
	}
}
