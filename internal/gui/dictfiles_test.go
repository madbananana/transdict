package gui

import (
	"os"
	"path/filepath"
	"testing"

	"transdict/internal/dict"
	"transdict/internal/settings"
)

// 导入词典 adds to the saved list, the first import also fills the folder box
// so 缺配置 never marks a service that will run, and removing drops one path.
func TestDictFilesAddAndRemove(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")

	dir := t.TempDir()
	first := filepath.Join(dir, "first.mdx")
	second := filepath.Join(dir, "second.mdx")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatalf("写入词典: %v", err)
		}
	}

	p, err := svc.AddProvider(settings.CatDict, "mdict")
	if err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	if err := svc.AddDictFiles(p.ID, []string{first, second, first}); err != nil {
		t.Fatalf("AddDictFiles: %v", err)
	}

	files, err := svc.DictFiles(p.ID)
	if err != nil {
		t.Fatalf("DictFiles: %v", err)
	}
	if len(files) != 2 || files[0] != first || files[1] != second {
		t.Fatalf("files = %v, want [%s %s]", files, first, second)
	}

	saved, ok := providerByID(store.Get(), p.ID)
	if !ok {
		t.Fatal("服务没有写进配置")
	}
	if got := saved.Get(dict.PathField); got != dir {
		t.Errorf("folder = %q, want %q", got, dir)
	}

	if err := svc.RemoveDictFile(p.ID, first); err != nil {
		t.Fatalf("RemoveDictFile: %v", err)
	}
	files, err = svc.DictFiles(p.ID)
	if err != nil {
		t.Fatalf("DictFiles: %v", err)
	}
	if len(files) != 1 || files[0] != second {
		t.Errorf("移除后 files = %v, want [%s]", files, second)
	}
}

func TestDictFilesRefuseUnknownService(t *testing.T) {
	svc := NewSettings(testStore(t), "")
	if _, err := svc.DictFiles("nope"); err == nil {
		t.Error("未知 id 的 DictFiles 应报错")
	}
	if err := svc.AddDictFiles("nope", []string{"/tmp/a.mdx"}); err == nil {
		t.Error("未知 id 的 AddDictFiles 应报错")
	}
	if err := svc.RemoveDictFile("nope", "/tmp/a.mdx"); err == nil {
		t.Error("未知 id 的 RemoveDictFile 应报错")
	}
}
