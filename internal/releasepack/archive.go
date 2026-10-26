package releasepack

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

const maxMemberSize = 128 << 20
const maxArchiveSize = 512 << 20

type Entry struct {
	Name, Path string
	Data       []byte
	Mode       int64
}
type FileRecord struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   int64  `json:"mode"`
}

func safeName(s string) bool {
	return s != "" && s != "." && s != ".." && path.Clean(s) == s && !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "../") && !strings.ContainsAny(s, "\\:\x00\r\n")
}
func (e Entry) open() (io.ReadCloser, int64, error) {
	if e.Path == "" {
		return io.NopCloser(bytes.NewReader(e.Data)), int64(len(e.Data)), nil
	}
	f, err := os.Open(e.Path)
	if err != nil {
		return nil, 0, err
	}
	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	if !s.Mode().IsRegular() {
		f.Close()
		return nil, 0, fmt.Errorf("not a regular input: %s", e.Path)
	}
	return f, s.Size(), nil
}
func record(r io.Reader, mode int64) (FileRecord, error) {
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(r, maxMemberSize+1))
	if err != nil {
		return FileRecord{}, err
	}
	if n > maxMemberSize {
		return FileRecord{}, fmt.Errorf("member exceeds %d bytes", maxMemberSize)
	}
	return FileRecord{SHA256: hex.EncodeToString(h.Sum(nil)), Size: n, Mode: mode}, nil
}
func entryRecord(e Entry) (FileRecord, error) {
	r, _, err := e.open()
	if err != nil {
		return FileRecord{}, err
	}
	defer r.Close()
	return record(r, e.Mode)
}
func hashFile(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// WriteArchive streams sorted regular members and normalizes metadata. It never
// creates a staging copy of the bundled survey or its reports.
func WriteArchive(name, format string, epoch int64, entries []Entry) (err error) {
	if format != "zip" {
		return fmt.Errorf("unsupported archive format %q", format)
	}
	entries = append([]Entry(nil), entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	for i, e := range entries {
		if !safeName(e.Name) || (i > 0 && entries[i-1].Name == e.Name) || (e.Mode != 0644 && e.Mode != 0755) {
			return fmt.Errorf("unsafe, duplicate or invalid archive member %q", e.Name)
		}
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if e := f.Close(); err == nil {
			err = e
		}
		if err != nil {
			os.Remove(name)
		}
	}()
	stamp := time.Unix(epoch, 0).UTC()
	write := func(e Entry, w io.Writer) error {
		r, n, err := e.open()
		if err != nil {
			return err
		}
		defer r.Close()
		copied, err := io.Copy(w, r)
		if err == nil && copied != n {
			return fmt.Errorf("source size changed: %s", e.Name)
		}
		return err
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.Name, Method: zip.Deflate, Modified: stamp}
		h.SetMode(os.FileMode(e.Mode))
		w, x := zw.CreateHeader(h)
		if x != nil {
			return x
		}
		if x = write(e, w); x != nil {
			return x
		}
	}
	return zw.Close()
}

// ReadArchive performs a separate pass over the compressed file. Consumers see
// only validated relative regular-file names, and must read within this callback.
func ReadArchive(name string, visit func(string, int64, io.Reader) error) (map[string]FileRecord, error) {
	records := map[string]FileRecord{}
	var total int64
	consume := func(n string, mode int64, size int64, r io.Reader) error {
		if len(records) >= 1000 {
			return fmt.Errorf("archive member count limit exceeded")
		}
		if !safeName(n) {
			return fmt.Errorf("unsafe archive path %q", n)
		}
		if _, ok := records[n]; ok {
			return fmt.Errorf("duplicate archive path %q", n)
		}
		if size < 0 || size > maxMemberSize || total+size > maxArchiveSize {
			return fmt.Errorf("archive size limit exceeded")
		}
		total += size
		if mode != 0644 && mode != 0755 {
			return fmt.Errorf("unexpected mode for %s", n)
		}
		h := sha256.New()
		limited := io.LimitReader(r, size+1)
		tr := io.TeeReader(limited, h)
		var consumed int64
		if visit != nil {
			c := &countReader{r: tr}
			if err := visit(n, mode, c); err != nil {
				return err
			}
			consumed = c.n
		}
		left, err := io.Copy(io.Discard, tr)
		if err != nil {
			return err
		}
		if left+consumed != size {
			return fmt.Errorf("incorrect size for %s", n)
		}
		records[n] = FileRecord{SHA256: hex.EncodeToString(h.Sum(nil)), Size: size, Mode: mode}
		return nil
	}
	if strings.HasSuffix(name, ".zip") {
		z, err := zip.OpenReader(name)
		if err != nil {
			return nil, err
		}
		defer z.Close()
		for _, m := range z.File {
			if !m.Mode().IsRegular() {
				return nil, fmt.Errorf("nonregular archive member %s", m.Name)
			}
			r, err := m.Open()
			if err != nil {
				return nil, err
			}
			err = consume(m.Name, int64(m.Mode().Perm()), int64(m.UncompressedSize64), r)
			closeErr := r.Close()
			if err != nil {
				return nil, err
			}
			if closeErr != nil {
				return nil, closeErr
			}
		}
	} else {
		return nil, fmt.Errorf("expected .zip archive")
	}
	return records, nil
}

type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) { n, e := c.r.Read(p); c.n += int64(n); return n, e }
