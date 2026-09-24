package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// UploadChunkSize 断点续传每个分片的最大字节数。
const UploadChunkSize = 8 << 20

const partSuffix = ".part"

// uploadSession 一次未完成的断点续传；已写入字节数即 .part 文件大小。
type uploadSession struct {
	mu        sync.Mutex
	name      string
	mime      string
	size      int64
	path      string
	updatedAt time.Time
}

type fileResponse struct {
	FileID string `json:"fileId"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Mime   string `json:"mime"`
}

type uploadStateResponse struct {
	UploadID  string        `json:"uploadId"`
	Offset    int64         `json:"offset"`
	Size      int64         `json:"size"`
	ChunkSize int64         `json:"chunkSize"`
	File      *fileResponse `json:"file,omitempty"`
}

// registerFile 登记已写完的文件，供 /api/files/{id} 下载。
func (s *Server) registerFile(id, name, mime, path string, size int64) fileResponse {
	s.files.Store(id, fileRecord{
		Name:      name,
		Size:      size,
		Mime:      mime,
		Path:      path,
		CreatedAt: time.Now(),
	})
	return fileResponse{FileID: id, Name: name, Size: size, Mime: mime}
}

func (s *Server) fileResponseFor(id string) (*fileResponse, bool) {
	raw, ok := s.files.Load(id)
	if !ok {
		return nil, false
	}
	rec := raw.(fileRecord)
	return &fileResponse{FileID: id, Name: rec.Name, Size: rec.Size, Mime: rec.Mime}, true
}

// handleUpload 单次 multipart 上传（兼容 curl）；流式写盘，不把文件读进内存。
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	maxUpload := s.cfg.MaxUploadBytes
	// multipart 头部开销留 1 MiB 余量
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)

	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "invalid multipart form", http.StatusBadRequest)
			return
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}

		id, err := randomID()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		destPath := filepath.Join(s.cfg.UploadDir, id)
		dest, err := os.Create(destPath)
		if err != nil {
			slog.Error("upload create file failed", "path", destPath, "err", err)
			http.Error(w, "cannot save file", http.StatusInternalServerError)
			return
		}

		// 多读 1 字节用于判断是否超限
		written, err := io.Copy(dest, io.LimitReader(part, maxUpload+1))
		_ = dest.Close()
		if err != nil || written > maxUpload {
			_ = os.Remove(destPath)
			if written > maxUpload || isMaxBytesError(err) {
				http.Error(w, fmt.Sprintf("file too large (max %d MiB)", maxUpload>>20), http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}

		resp := s.registerFile(id, sanitizeFileName(part.FileName()), normalizeMime(part.Header.Get("Content-Type")), destPath, written)
		writeJSON(w, http.StatusOK, resp)
		return
	}
}

// handleUploadCreate POST /api/uploads：创建断点续传会话。
func (s *Server) handleUploadCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		Mime string `json:"mime"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Size < 0 {
		http.Error(w, "invalid size", http.StatusBadRequest)
		return
	}
	if req.Size > s.cfg.MaxUploadBytes {
		http.Error(w, fmt.Sprintf("file too large (max %d MiB)", s.cfg.MaxUploadBytes>>20), http.StatusRequestEntityTooLarge)
		return
	}

	id, err := randomID()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	sess := &uploadSession{
		name:      sanitizeFileName(req.Name),
		mime:      normalizeMime(req.Mime),
		size:      req.Size,
		path:      filepath.Join(s.cfg.UploadDir, id+partSuffix),
		updatedAt: time.Now(),
	}
	f, err := os.Create(sess.path)
	if err != nil {
		slog.Error("upload create part failed", "path", sess.path, "err", err)
		http.Error(w, "cannot save file", http.StatusInternalServerError)
		return
	}
	_ = f.Close()

	resp := uploadStateResponse{UploadID: id, Size: req.Size, ChunkSize: UploadChunkSize}
	if req.Size == 0 {
		file, err := s.finishUpload(id, sess)
		if err != nil {
			http.Error(w, "cannot save file", http.StatusInternalServerError)
			return
		}
		resp.File = file
	} else {
		s.uploads.Store(id, sess)
	}
	writeJSON(w, http.StatusCreated, resp)
}

// handleUploadSession GET/PUT/DELETE /api/uploads/{id}
func (s *Server) handleUploadSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/uploads/")
	if !isHexFileID(id) {
		http.NotFound(w, r)
		return
	}

	raw, ok := s.uploads.Load(id)
	if !ok {
		// 已完成的上传：客户端可能没收到完成响应，返回文件信息让它继续
		if file, done := s.fileResponseFor(id); done && r.Method != http.MethodDelete {
			writeJSON(w, http.StatusOK, uploadStateResponse{UploadID: id, Offset: file.Size, Size: file.Size, ChunkSize: UploadChunkSize, File: file})
			return
		}
		http.Error(w, "upload not found or expired", http.StatusNotFound)
		return
	}
	sess := raw.(*uploadSession)

	switch r.Method {
	case http.MethodGet:
		sess.mu.Lock()
		offset, err := fileSize(sess.path)
		sess.mu.Unlock()
		if err != nil {
			http.Error(w, "upload not found or expired", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, uploadStateResponse{UploadID: id, Offset: offset, Size: sess.size, ChunkSize: UploadChunkSize})
	case http.MethodPut:
		s.putChunk(w, r, id, sess)
	case http.MethodDelete:
		sess.mu.Lock()
		s.uploads.Delete(id)
		_ = os.Remove(sess.path)
		sess.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// putChunk 在 offset 处追加一个分片；offset 必须等于已写入的字节数。
func (s *Server) putChunk(w http.ResponseWriter, r *http.Request, id string, sess *uploadSession) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	current, err := fileSize(sess.path)
	if err != nil {
		http.Error(w, "upload not found or expired", http.StatusNotFound)
		return
	}
	offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if err != nil || offset != current {
		writeJSON(w, http.StatusConflict, uploadStateResponse{UploadID: id, Offset: current, Size: sess.size, ChunkSize: UploadChunkSize})
		return
	}

	limit := min(int64(UploadChunkSize), sess.size-current)
	f, err := os.OpenFile(sess.path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		http.Error(w, "cannot save file", http.StatusInternalServerError)
		return
	}
	// 多读 1 字节判断分片是否越界；中途断开时已写入的前缀保留，客户端据 offset 续传
	written, copyErr := io.Copy(f, io.LimitReader(r.Body, limit+1))
	closeErr := f.Close()
	sess.updatedAt = time.Now()

	if written > limit {
		_ = os.Truncate(sess.path, current)
		http.Error(w, "chunk exceeds declared size", http.StatusRequestEntityTooLarge)
		return
	}
	if copyErr != nil || closeErr != nil {
		slog.Warn("upload chunk interrupted", "id", id, "written", written, "err", errors.Join(copyErr, closeErr))
		http.Error(w, "chunk interrupted", http.StatusBadRequest)
		return
	}

	newOffset := current + written
	resp := uploadStateResponse{UploadID: id, Offset: newOffset, Size: sess.size, ChunkSize: UploadChunkSize}
	if newOffset == sess.size {
		file, err := s.finishUpload(id, sess)
		if err != nil {
			http.Error(w, "cannot save file", http.StatusInternalServerError)
			return
		}
		resp.File = file
	}
	writeJSON(w, http.StatusOK, resp)
}

// finishUpload 把 .part 转为正式文件并登记；调用方需持有 sess.mu（或会话尚未公开）。
func (s *Server) finishUpload(id string, sess *uploadSession) (*fileResponse, error) {
	final := filepath.Join(s.cfg.UploadDir, id)
	if err := os.Rename(sess.path, final); err != nil {
		slog.Error("upload finalize failed", "id", id, "err", err)
		return nil, err
	}
	s.uploads.Delete(id)
	file := s.registerFile(id, sess.name, sess.mime, final, sess.size)
	return &file, nil
}

func (s *Server) purgeStaleUploads(cutoff time.Time) {
	s.uploads.Range(func(key, value any) bool {
		sess := value.(*uploadSession)
		sess.mu.Lock()
		if sess.updatedAt.Before(cutoff) {
			s.uploads.Delete(key)
			_ = os.Remove(sess.path)
		}
		sess.mu.Unlock()
		return true
	})
}

// removeOrphanUploads 删除上次运行遗留的上传文件（内存索引已丢失，无法再访问）。
// 只匹配本程序的命名（32 位 hex，可带 .part），不碰目录里的其他文件。
func removeOrphanUploads(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	removed := 0
	for _, e := range entries {
		if !e.Type().IsRegular() || !isHexFileID(strings.TrimSuffix(e.Name(), partSuffix)) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
			removed++
		}
	}
	if removed > 0 {
		slog.Info("removed orphan uploads", "count", removed, "dir", dir)
	}
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "" || name == "." || name == "/" {
		return "file"
	}
	for len(name) > 255 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name
}

func normalizeMime(mime string) string {
	mime = strings.TrimSpace(mime)
	if mime == "" || len(mime) > 255 {
		return "application/octet-stream"
	}
	return mime
}

func isMaxBytesError(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
