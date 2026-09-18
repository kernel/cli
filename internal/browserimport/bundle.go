package browserimport

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/klauspost/compress/zstd"
)

const maxBundleBytes = 128 << 20

type Manifest struct {
	Version  int             `json:"version"`
	Source   BundleSource    `json:"source"`
	Profiles []BundleProfile `json:"profiles,omitempty"`
}

type BundleSource struct {
	OS            string `json:"os"`
	HelperVersion string `json:"helper_version"`
}

type BundleProfile struct {
	ID         string       `json:"id"`
	Browser    string       `json:"browser"`
	SourceName string       `json:"source_name"`
	TargetName string       `json:"target_name"`
	Files      ProfileFiles `json:"files"`
}

type ProfileFiles struct {
	Cookies string `json:"cookies,omitempty"`
}

func BuildCookieBundle(ctx context.Context, profile Profile, targetName, version string, cookies []Cookie) ([]byte, error) {
	if len(cookies) == 0 {
		return nil, fmt.Errorf("no cookies were selected")
	}
	cookiePath := "profiles/" + profile.ID + "/cookies.jsonl"
	manifest := Manifest{
		Version: BundleVersion,
		Source:  BundleSource{OS: "macos", HelperVersion: version},
		Profiles: []BundleProfile{{
			ID: profile.ID, Browser: profile.Browser.ID, SourceName: profile.DisplayName(), TargetName: targetName,
			Files: ProfileFiles{Cookies: cookiePath},
		}},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode import manifest: %w", err)
	}
	var cookieData bytes.Buffer
	encoder := json.NewEncoder(&cookieData)
	for _, cookie := range cookies {
		if err := encoder.Encode(cookie); err != nil {
			return nil, fmt.Errorf("encode browser cookie: %w", err)
		}
	}
	return encodeBundle(ctx, manifestData, map[string][]byte{cookiePath: cookieData.Bytes()})
}

func encodeBundle(ctx context.Context, manifest []byte, files map[string][]byte) ([]byte, error) {
	var output bytes.Buffer
	zstdWriter, err := zstd.NewWriter(&output, zstd.WithEncoderConcurrency(1))
	if err != nil {
		return nil, err
	}
	tarWriter := tar.NewWriter(zstdWriter)
	write := func(name string, data []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err := tarWriter.Write(data)
		return err
	}
	if err := write("manifest.json", manifest); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := write(path, files[path]); err != nil {
			return nil, err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return nil, err
	}
	if err := zstdWriter.Close(); err != nil {
		return nil, err
	}
	if output.Len() > maxBundleBytes {
		return nil, fmt.Errorf("selected browser data exceeds the 128 MiB import limit")
	}
	return output.Bytes(), nil
}
