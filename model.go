package main

import (
	"archive/tar"
	"compress/bzip2"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	modelName = "kokoro-multi-lang-v1_0"
	modelURL  = "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/" + modelName + ".tar.bz2"
)

func modelDir() (string, error) {
	if d := os.Getenv("CLUMYAP_MODEL_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "clumyap"), nil
}

// ensureModel returns the model folder, downloading and unpacking it on first use.
func ensureModel(quiet bool) (string, error) {
	base, err := modelDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, modelName)
	if _, err := os.Stat(filepath.Join(dir, "model.onnx")); err == nil {
		return dir, nil
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	logf(quiet, "First run: downloading the Kokoro model (~350 MB) to %s\n", base)

	resp, err := http.Get(modelURL)
	if err != nil {
		return "", fmt.Errorf("download model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download model: %s", resp.Status)
	}

	// Unpack into a temp folder and rename at the end, so an interrupted download never looks complete.
	tmp, err := os.MkdirTemp(base, ".download-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	body := io.Reader(resp.Body)
	if !quiet {
		body = &progressReader{r: resp.Body, total: resp.ContentLength}
	}
	if err := untar(bzip2.NewReader(body), tmp); err != nil {
		return "", fmt.Errorf("unpack model: %w", err)
	}
	logf(quiet, "\n")
	if err := os.Rename(filepath.Join(tmp, modelName), dir); err != nil {
		return "", err
	}
	return dir, nil
}

func untar(r io.Reader, dst string) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dst, h.Name)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		}
	}
}

type progressReader struct {
	r           io.Reader
	total, read int64
	lastPct     int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	if p.total > 0 {
		if pct := p.read * 100 / p.total; pct != p.lastPct {
			p.lastPct = pct
			fmt.Fprintf(os.Stderr, "\r  %3d%%  %d/%d MB", pct, p.read>>20, p.total>>20)
		}
	}
	return n, err
}
