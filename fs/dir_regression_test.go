package fs

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestListFilesHiddenRecursiveRegression(t *testing.T) {
	dir := t.TempDir()
	all := []string{".hidden", "visible", "sub/.hidden", "sub/visible", "sub/deep/.hidden", "sub/deep/visible", "sub/.hidden-dir/visible"}
	for _, name := range all {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, hide := range []bool{true, false} {
		got, err := ListFiles(dir, 3, hide)
		if err != nil {
			t.Fatal(err)
		}
		want := all
		if hide {
			want = []string{"visible", "sub/visible", "sub/deep/visible"}
		}
		absolute := make([]string, len(want))
		for i, name := range want {
			absolute[i] = filepath.Join(dir, name)
		}
		slices.Sort(got)
		slices.Sort(absolute)
		if !reflect.DeepEqual(got, absolute) {
			t.Fatalf("hideHidden=%v: got %v, want %v", hide, got, absolute)
		}
	}
}
