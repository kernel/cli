package browserimport

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/klauspost/compress/zstd"
)

const (
	maxBundleBytes           = 128 << 20
	maxPortableFileBytes     = 64 << 20
	maxPortableRecordBytes   = 1 << 20
	maxPortableRecords       = 100_000
	maxPortableDocumentBytes = 16 << 20
)

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
	Cookies   string `json:"cookies,omitempty"`
	Storage   string `json:"storage,omitempty"`
	Bookmarks string `json:"bookmarks,omitempty"`
	History   string `json:"history,omitempty"`
}

type bundleFile struct {
	path string
	data []byte
}

func BuildProfileBundle(ctx context.Context, profile Profile, targetName, version string, data ProfileData) ([]byte, error) {
	if len(data.Cookies) == 0 && len(data.Storage) == 0 && data.Bookmarks == nil && len(data.History) == 0 {
		return nil, fmt.Errorf("no browser data was selected")
	}
	files := ProfileFiles{}
	payloads := make([]bundleFile, 0, 5)
	addJSON := func(path, label string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode browser %s: %w", label, err)
		}
		if len(encoded) > maxPortableDocumentBytes {
			return fmt.Errorf("browser %s exceeds the 16 MiB import limit", label)
		}
		payloads = append(payloads, bundleFile{path: path, data: encoded})
		return nil
	}
	if len(data.Cookies) > 0 {
		files.Cookies = "profiles/" + profile.ID + "/cookies.jsonl"
		encoded, err := encodeJSONL("cookies", data.Cookies)
		if err != nil {
			return nil, fmt.Errorf("encode browser cookies: %w", err)
		}
		payloads = append(payloads, bundleFile{path: files.Cookies, data: encoded})
	}
	if len(data.Storage) > 0 {
		files.Storage = "profiles/" + profile.ID + "/storage.jsonl"
		encoded, err := encodeJSONL("local storage", data.Storage)
		if err != nil {
			return nil, fmt.Errorf("encode browser local storage: %w", err)
		}
		payloads = append(payloads, bundleFile{path: files.Storage, data: encoded})
	}
	if data.Bookmarks != nil {
		files.Bookmarks = "profiles/" + profile.ID + "/bookmarks.json"
		if err := addJSON(files.Bookmarks, "bookmarks", data.Bookmarks); err != nil {
			return nil, err
		}
	}
	if len(data.History) > 0 {
		files.History = "profiles/" + profile.ID + "/history.jsonl"
		encoded, err := encodeJSONL("history", data.History)
		if err != nil {
			return nil, fmt.Errorf("encode browser history: %w", err)
		}
		payloads = append(payloads, bundleFile{path: files.History, data: encoded})
	}
	manifest := Manifest{
		Version: BundleVersion,
		Source:  BundleSource{OS: "macos", HelperVersion: version},
		Profiles: []BundleProfile{{
			ID: profile.ID, Browser: profile.Browser.ID, SourceName: profile.DisplayName(), TargetName: targetName,
			Files: files,
		}},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode import manifest: %w", err)
	}
	return encodeBundle(ctx, manifestData, payloads)
}

func encodeJSONL[T any](label string, records []T) ([]byte, error) {
	if len(records) > maxPortableRecords {
		return nil, fmt.Errorf("browser %s exceeds the %d record import limit", label, maxPortableRecords)
	}
	var output bytes.Buffer
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		if len(encoded)+1 > maxPortableRecordBytes {
			return nil, fmt.Errorf("one browser %s record exceeds the 1 MiB import limit", label)
		}
		if output.Len()+len(encoded)+1 > maxPortableFileBytes {
			return nil, fmt.Errorf("browser %s exceeds the 64 MiB import limit", label)
		}
		output.Write(encoded)
		output.WriteByte('\n')
	}
	return output.Bytes(), nil
}

func encodeBundle(ctx context.Context, manifest []byte, files []bundleFile) ([]byte, error) {
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
	for _, file := range files {
		if err := write(file.path, file.data); err != nil {
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
