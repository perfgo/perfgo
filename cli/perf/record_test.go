package perf

import (
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRemoteBinaryCopier struct {
	data  []byte
	err   error
	calls int
}

func (f *fakeRemoteBinaryCopier) CopyFromRemote(_ string, dst io.Writer) error {
	f.calls++
	for offset := 0; offset < len(f.data); offset += 7 {
		end := min(offset+7, len(f.data))
		if _, err := dst.Write(f.data[offset:end]); err != nil {
			return err
		}
	}
	return f.err
}

func TestCopyBinaryFromRemote(t *testing.T) {
	data := []byte(strings.Repeat("prometheus-binary-data", 32))
	hash := testBinaryHash(data)
	localPath := filepath.Join(t.TempDir(), "prometheus.binary")
	copier := &fakeRemoteBinaryCopier{data: data}

	err := copyBinaryFromRemote(zerolog.Nop(), copier, "/bin/prometheus", localPath, hash, uint64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, 1, copier.calls)

	written, err := os.ReadFile(localPath)
	require.NoError(t, err)
	assert.Equal(t, data, written)
	info, err := os.Stat(localPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestCopyBinaryFromRemoteRemovesPartialFileOnMismatch(t *testing.T) {
	data := []byte("truncated binary")
	dir := t.TempDir()
	localPath := filepath.Join(dir, "prometheus.binary")
	copier := &fakeRemoteBinaryCopier{data: data}

	err := copyBinaryFromRemote(zerolog.Nop(), copier, "/bin/prometheus", localPath, testBinaryHash(data), uint64(len(data)+1))
	require.ErrorContains(t, err, "size mismatch")
	assert.NoFileExists(t, localPath)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestCopyBinaryFromRemoteRemovesPartialFileOnHashMismatch(t *testing.T) {
	data := []byte("remote binary")
	dir := t.TempDir()
	localPath := filepath.Join(dir, "prometheus.binary")
	copier := &fakeRemoteBinaryCopier{data: data}

	err := copyBinaryFromRemote(zerolog.Nop(), copier, "/bin/prometheus", localPath, testBinaryHash([]byte("other content")), uint64(len(data)))
	require.ErrorContains(t, err, "hash mismatch")
	assert.NoFileExists(t, localPath)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestCopyBinaryFromRemoteRemovesPartialFileOnRemoteError(t *testing.T) {
	data := []byte("partial binary")
	dir := t.TempDir()
	localPath := filepath.Join(dir, "prometheus.binary")
	copier := &fakeRemoteBinaryCopier{data: data, err: errors.New("connection lost")}

	err := copyBinaryFromRemote(zerolog.Nop(), copier, "/bin/prometheus", localPath, testBinaryHash(data), uint64(len(data)))
	require.ErrorContains(t, err, "connection lost")
	assert.NoFileExists(t, localPath)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFormatBytes(t *testing.T) {
	tests := map[uint64]string{
		0:                      "0 B",
		1023:                   "1023 B",
		1024:                   "1.0 KiB",
		5 * 1024 * 1024:        "5.0 MiB",
		3 * 1024 * 1024 * 1024: "3.0 GiB",
	}

	for size, expected := range tests {
		assert.Equal(t, expected, formatBytes(size))
	}
}

func TestFormatTransferRate(t *testing.T) {
	assert.Equal(t, "2.0 MiB/s", formatTransferRate(4*1024*1024, 2*time.Second))
	assert.Equal(t, "0 B/s", formatTransferRate(1024, 0))
}

func TestReuseCachedBinary(t *testing.T) {
	historyDir := t.TempDir()
	previousRun := filepath.Join(historyDir, "previous")
	currentRun := filepath.Join(historyDir, "current")
	require.NoError(t, os.Mkdir(previousRun, 0755))
	require.NoError(t, os.Mkdir(currentRun, 0755))

	data := []byte(strings.Repeat("cached-binary", 16))
	hash := testBinaryHash(data)
	filename := hash + ".prometheus.binary"
	require.NoError(t, os.WriteFile(filepath.Join(previousRun, filename), data, 0755))
	localPath := filepath.Join(currentRun, filename)

	reused := reuseCachedBinary(zerolog.Nop(), currentRun, filename, localPath, hash, uint64(len(data)))
	assert.True(t, reused)
	written, err := os.ReadFile(localPath)
	require.NoError(t, err)
	assert.Equal(t, data, written)
}

func TestReuseCachedBinaryRejectsCorruptArtifact(t *testing.T) {
	historyDir := t.TempDir()
	previousRun := filepath.Join(historyDir, "previous")
	currentRun := filepath.Join(historyDir, "current")
	require.NoError(t, os.Mkdir(previousRun, 0755))
	require.NoError(t, os.Mkdir(currentRun, 0755))

	expected := []byte("expected-binary")
	corrupt := []byte("corrupted-binar")
	hash := testBinaryHash(expected)
	filename := hash + ".prometheus.binary"
	require.Equal(t, len(expected), len(corrupt))
	require.NoError(t, os.WriteFile(filepath.Join(previousRun, filename), corrupt, 0755))
	localPath := filepath.Join(currentRun, filename)

	reused := reuseCachedBinary(zerolog.Nop(), currentRun, filename, localPath, hash, uint64(len(expected)))
	assert.False(t, reused)
	assert.NoFileExists(t, localPath)
}

func testBinaryHash(data []byte) string {
	hash := sha256.Sum256(data)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(hash[:]))
}
