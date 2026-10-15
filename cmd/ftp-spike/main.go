// ftp-spike exercises an explicitly configured FTP/FTPS account without touching existing files.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"strconv"
	"time"

	"quickftp/internal/storage"
	"quickftp/internal/transfer"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	port, _ := strconv.Atoi(env("QUICKFTP_FTP_PORT", "21"))
	tlsVerify := env("QUICKFTP_FTP_TLS_VERIFY", "true") != "false"
	client := &transfer.FTPClient{Config: transfer.FTPConfig{Host: os.Getenv("QUICKFTP_FTP_HOST"), Port: port, Protocol: storage.Protocol(env("QUICKFTP_FTP_PROTOCOL", "ftps_explicit")), Username: os.Getenv("QUICKFTP_FTP_USERNAME"), Password: os.Getenv("QUICKFTP_FTP_PASSWORD"), TLSVerify: tlsVerify, Timeout: 15 * time.Second}}
	if client.Config.Host == "" || client.Config.Username == "" || client.Config.Password == "" {
		fatal("set QUICKFTP_FTP_HOST, QUICKFTP_FTP_USERNAME, and QUICKFTP_FTP_PASSWORD")
	}
	root := env("QUICKFTP_FTP_REMOTE_ROOT", "/")
	if _, err := client.List(ctx, root); err != nil {
		fatal("LIST failed: %v", err)
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		fatal("generate probe name: %v", err)
	}
	base := ".quickftp-spike-" + hex.EncodeToString(nonce[:])
	original, renamed := path.Join(root, base+".tmp"), path.Join(root, base+".renamed")
	payload := []byte("QuickFTP FTP/FTPS transfer probe")
	if err := client.Upload(ctx, original, bytes.NewReader(payload)); err != nil {
		fatal("STOR failed: %v", err)
	}
	listing, err := client.List(ctx, root)
	if err != nil {
		_ = client.Delete(context.Background(), original)
		fatal("LIST after upload failed: %v", err)
	}
	found := false
	for _, f := range listing {
		if f.Path == original {
			found = true
			break
		}
	}
	if !found {
		_ = client.Delete(context.Background(), original)
		fatal("uploaded probe was not present in LIST")
	}
	data, err := download(ctx, client, original)
	if err != nil {
		_ = client.Delete(context.Background(), original)
		fatal("RETR failed: %v", err)
	}
	if !bytes.Equal(data, payload) {
		_ = client.Delete(context.Background(), original)
		fatal("downloaded probe content did not match")
	}
	if err := client.Rename(ctx, original, renamed); err != nil {
		_ = client.Delete(context.Background(), original)
		fatal("RNFR/RNTO failed: %v", err)
	}
	if err := client.Delete(ctx, renamed); err != nil {
		fatal("DELE failed for %s: %v", renamed, err)
	}
	fmt.Println("FTP/FTPS login, protected listing, upload, download, rename, and delete succeeded")
}

func download(ctx context.Context, c *transfer.FTPClient, name string) ([]byte, error) {
	var b bytes.Buffer
	err := c.Download(ctx, name, &b)
	return b.Bytes(), err
}
func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func fatal(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...); os.Exit(1) }
