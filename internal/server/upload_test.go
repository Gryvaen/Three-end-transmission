package server

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, cfg Config) (*Server, *httptest.Server) {
	t.Helper()
	if cfg.UploadDir == "" {
		cfg.UploadDir = t.TempDir()
	}
	if cfg.MaxUploadBytes == 0 {
		cfg.MaxUploadBytes = 64 << 20
	}
	if cfg.Retention == 0 {
		cfg.Retention = time.Hour
	}
	srv := New(cfg)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func doJSON(t *testing.T, method, url string, body any, out any) int {
	t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		r = bytes.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		r = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, url, r)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	if out != nil && strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func download(t *testing.T, url string) []byte {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download %s: HTTP %d", url, resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	return data
}

func TestResumableUpload(t *testing.T) {
	_, ts := newTestServer(t, Config{})

	data := make([]byte, 2*UploadChunkSize+12345)
	_, _ = rand.Read(data)

	var st uploadStateResponse
	if code := doJSON(t, http.MethodPost, ts.URL+"/api/uploads",
		map[string]any{"name": "../video.mp4", "size": len(data), "mime": "video/mp4"}, &st); code != http.StatusCreated {
		t.Fatalf("create: HTTP %d", code)
	}
	sessURL := ts.URL + "/api/uploads/" + st.UploadID

	// 第一片
	if code := doJSON(t, http.MethodPut, sessURL+"?offset=0", data[:UploadChunkSize], &st); code != http.StatusOK || st.Offset != UploadChunkSize {
		t.Fatalf("chunk 1: HTTP %d offset %d", code, st.Offset)
	}

	// offset 不匹配：409 并告知正确 offset
	var conflict uploadStateResponse
	if code := doJSON(t, http.MethodPut, sessURL+"?offset=0", data[:10], &conflict); code != http.StatusConflict || conflict.Offset != UploadChunkSize {
		t.Fatalf("conflict: HTTP %d offset %d", code, conflict.Offset)
	}

	// 断线后通过 GET 查询进度再继续
	var state uploadStateResponse
	if code := doJSON(t, http.MethodGet, sessURL, nil, &state); code != http.StatusOK || state.Offset != UploadChunkSize {
		t.Fatalf("state: HTTP %d offset %d", code, state.Offset)
	}

	for off := state.Offset; off < int64(len(data)); {
		end := min(off+UploadChunkSize, int64(len(data)))
		if code := doJSON(t, http.MethodPut, fmt.Sprintf("%s?offset=%d", sessURL, off), data[off:end], &st); code != http.StatusOK {
			t.Fatalf("chunk at %d: HTTP %d", off, code)
		}
		off = st.Offset
	}
	if st.File == nil || st.File.Size != int64(len(data)) || st.File.Name != "video.mp4" {
		t.Fatalf("unexpected completion: %+v", st.File)
	}

	if got := download(t, ts.URL+"/api/files/"+st.File.FileID); !bytes.Equal(got, data) {
		t.Fatal("downloaded content differs")
	}

	// 完成后再查询：返回文件信息（客户端丢失完成响应时可恢复）
	var after uploadStateResponse
	if code := doJSON(t, http.MethodGet, sessURL, nil, &after); code != http.StatusOK || after.File == nil {
		t.Fatalf("after complete: HTTP %d file %+v", code, after.File)
	}
}

func TestUploadChunkBeyondSizeRejected(t *testing.T) {
	srv, ts := newTestServer(t, Config{})

	var st uploadStateResponse
	doJSON(t, http.MethodPost, ts.URL+"/api/uploads", map[string]any{"name": "a.txt", "size": 10}, &st)
	sessURL := ts.URL + "/api/uploads/" + st.UploadID

	if code := doJSON(t, http.MethodPut, sessURL+"?offset=0", bytes.Repeat([]byte("x"), 20), nil); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", code)
	}
	var state uploadStateResponse
	doJSON(t, http.MethodGet, sessURL, nil, &state)
	if state.Offset != 0 {
		t.Fatalf("offset should be rolled back to 0, got %d", state.Offset)
	}

	if code := doJSON(t, http.MethodDelete, sessURL, nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: HTTP %d", code)
	}
	if _, err := os.Stat(filepath.Join(srv.cfg.UploadDir, st.UploadID+partSuffix)); !os.IsNotExist(err) {
		t.Fatal(".part file should be removed after delete")
	}
}

func TestUploadCreateLimits(t *testing.T) {
	_, ts := newTestServer(t, Config{MaxUploadBytes: 1 << 20})

	if code := doJSON(t, http.MethodPost, ts.URL+"/api/uploads", map[string]any{"name": "big", "size": 2 << 20}, nil); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", code)
	}

	var st uploadStateResponse
	if code := doJSON(t, http.MethodPost, ts.URL+"/api/uploads", map[string]any{"name": "empty.txt", "size": 0}, &st); code != http.StatusCreated || st.File == nil {
		t.Fatalf("empty file should complete immediately: HTTP %d %+v", code, st)
	}
}

func TestMultipartUploadStreams(t *testing.T) {
	_, ts := newTestServer(t, Config{MaxUploadBytes: 1 << 20})

	post := func(content []byte) (int, fileResponse) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("note", "ignored")
		fw, _ := mw.CreateFormFile("file", "hello.txt")
		_, _ = fw.Write(content)
		_ = mw.Close()

		resp, err := http.Post(ts.URL+"/api/upload", mw.FormDataContentType(), &buf)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var fr fileResponse
		_ = json.NewDecoder(resp.Body).Decode(&fr)
		return resp.StatusCode, fr
	}

	code, fr := post([]byte("hello lan"))
	if code != http.StatusOK || fr.Name != "hello.txt" {
		t.Fatalf("upload: HTTP %d %+v", code, fr)
	}
	if got := download(t, ts.URL+"/api/files/"+fr.FileID); string(got) != "hello lan" {
		t.Fatalf("got %q", got)
	}

	if code, _ := post(make([]byte, (1<<20)+1)); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize: want 413, got %d", code)
	}
}

func TestRemoveOrphanUploads(t *testing.T) {
	dir := t.TempDir()
	orphan := strings.Repeat("a", 32)
	for _, name := range []string{orphan, orphan + partSuffix, "keep.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	New(Config{UploadDir: dir, Retention: time.Hour})

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "keep.txt" {
		t.Fatalf("unexpected dir contents: %v", entries)
	}
}

func TestPurgeExpired(t *testing.T) {
	srv, ts := newTestServer(t, Config{})

	var st uploadStateResponse
	doJSON(t, http.MethodPost, ts.URL+"/api/uploads", map[string]any{"name": "a", "size": 5}, &st)
	var done uploadStateResponse
	doJSON(t, http.MethodPost, ts.URL+"/api/uploads", map[string]any{"name": "b", "size": 3}, &done)
	doJSON(t, http.MethodPut, ts.URL+"/api/uploads/"+done.UploadID+"?offset=0", []byte("abc"), &done)

	srv.purgeExpiredFiles(-time.Minute) // cutoff 在未来：全部过期

	if code := doJSON(t, http.MethodGet, ts.URL+"/api/uploads/"+st.UploadID, nil, nil); code != http.StatusNotFound {
		t.Fatalf("stale session: want 404, got %d", code)
	}
	if code := doJSON(t, http.MethodGet, ts.URL+"/api/files/"+done.File.FileID, nil, nil); code != http.StatusNotFound {
		t.Fatalf("expired file: want 404, got %d", code)
	}
	entries, _ := os.ReadDir(srv.cfg.UploadDir)
	if len(entries) != 0 {
		t.Fatalf("upload dir should be empty, got %v", entries)
	}
}
