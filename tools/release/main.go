// Command release builds the openai-to-qwen OCI image without a Docker daemon
// and pushes it to a container registry (default: Aliyun ACR).
//
// Modes:
//
//	go run ./tools/release                 # push (needs ACR_USERNAME/ACR_PASSWORD)
//	go run ./tools/release -tarball out.tar # write docker-archive locally
//	go run ./tools/release -extract dir     # extract rootfs for verification
package main

import (
	"archive/tar"
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

func main() {
	var (
		tarballOut = flag.String("tarball", "", "write docker-archive to this file instead of pushing")
		extractDir = flag.String("extract", "", "extract image rootfs to this directory instead of pushing")
		version    = flag.String("version", "", "image tag (default: VERSION env or v1.0.0)")
	)
	flag.Parse()

	registry := getenv("IMAGE_REGISTRY", "registry.cn-hangzhou.aliyuncs.com/liugangqiang/openai-to-qwen")
	ver := *version
	if ver == "" {
		ver = getenv("VERSION", "v1.0.0")
	}
	binary := getenv("BINARY", "dist/openai-to-qwen")
	cacertFile := getenv("CACERT", "tools/release/ca-certificates.crt")

	if _, err := os.Stat(cacertFile); os.IsNotExist(err) {
		if err := exportSystemCACerts(cacertFile); err != nil {
			log.Fatalf("export CA certs: %v", err)
		}
		log.Printf("exported CA bundle to %s", cacertFile)
	}

	layerTar, err := buildRootfsLayer(binary, cacertFile)
	if err != nil {
		log.Fatalf("build layer: %v", err)
	}
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(layerTar)), nil
	})
	if err != nil {
		log.Fatalf("layer: %v", err)
	}

	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		log.Fatalf("append layers: %v", err)
	}

	diffID, err := layer.DiffID()
	if err != nil {
		log.Fatalf("diffid: %v", err)
	}

	cfg := &v1.ConfigFile{
		Architecture: "amd64",
		OS:           "linux",
		Created:      v1.Time{Time: time.Now()},
		RootFS: v1.RootFS{
			Type:    "layers",
			DiffIDs: []v1.Hash{diffID},
		},
		Config: v1.Config{
			Entrypoint:   []string{"/openai-to-qwen"},
			WorkingDir:   "/",
			User:         "10001",
			ExposedPorts: map[string]struct{}{"8080/tcp": {}},
			Env: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
		},
	}
	img, err = mutate.ConfigFile(img, cfg)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if *extractDir != "" {
		if err := extractRootfs(img, *extractDir); err != nil {
			log.Fatalf("extract: %v", err)
		}
		log.Printf("extracted rootfs to %s", *extractDir)
		return
	}

	if *tarballOut != "" {
		ref, err := name.ParseReference(registry + ":" + ver)
		if err != nil {
			log.Fatalf("ref: %v", err)
		}
		if err := tarball.WriteToFile(*tarballOut, ref, img); err != nil {
			log.Fatalf("write tarball: %v", err)
		}
		log.Printf("wrote docker-archive to %s (%s)", *tarballOut, ref.Name())
		return
	}

	username := os.Getenv("ACR_USERNAME")
	password := os.Getenv("ACR_PASSWORD")
	if username == "" || password == "" {
		log.Fatal("ACR_USERNAME and ACR_PASSWORD must be set to push")
	}
	auth := authn.FromConfig(authn.AuthConfig{Username: username, Password: password})

	for _, tag := range []string{ver, "latest"} {
		ref, err := name.ParseReference(registry + ":" + tag)
		if err != nil {
			log.Fatalf("ref: %v", err)
		}
		start := time.Now()
		if err := remote.Write(ref, img, remote.WithAuth(auth), remote.WithUserAgent("openai-to-qwen-release")); err != nil {
			log.Fatalf("push %s: %v", ref.Name(), err)
		}
		log.Printf("pushed %s in %s", ref.Name(), time.Since(start))
	}
}

func buildRootfsLayer(binary, cacertFile string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := addTarFile(tw, binary, "openai-to-qwen", 0o755); err != nil {
		return nil, err
	}
	if err := addTarFile(tw, cacertFile, "etc/ssl/certs/ca-certificates.crt", 0o644); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func addTarFile(tw *tar.Writer, src, name string, mode int64) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name:     name,
		Mode:     mode,
		Size:     int64(len(data)),
		ModTime:  time.Unix(0, 0),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}

func extractRootfs(img v1.Image, dir string) error {
	rc := mutate.Extract(img)
	defer rc.Close()
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dir, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			_ = f.Close()
		}
	}
	return nil
}

func exportSystemCACerts(path string) error {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	seen := make(map[string]bool)
	for _, der := range pool.Subjects() {
		key := string(der)
		if seen[key] {
			continue
		}
		seen[key] = true
		_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	if buf.Len() == 0 {
		return fmt.Errorf("no CA certs found in system store")
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}