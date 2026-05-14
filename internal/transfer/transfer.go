package transfer

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"lunarbridge/internal/config"
)

const maxMemory = 32 << 20

type Service struct {
	store *config.Store
}

type SendItem struct {
	FieldName    string `json:"fieldName"`
	Name         string `json:"name"`
	RelativePath string `json:"relativePath"`
	Size         int64  `json:"size"`
}

type Manifest struct {
	SenderID   string     `json:"senderId"`
	SenderName string     `json:"senderName"`
	Items      []SendItem `json:"items"`
}

type Result struct {
	Saved int      `json:"saved"`
	Files []string `json:"files"`
}

func New(store *config.Store) *Service {
	return &Service{store: store}
}

func (s *Service) Send(peer config.Peer, files []*multipart.FileHeader) error {
	cfg := s.store.Snapshot()
	if len(files) == 0 {
		return errors.New("choose at least one file")
	}
	bodyReader, bodyWriter := io.Pipe()
	mp := multipart.NewWriter(bodyWriter)
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		defer bodyWriter.Close()
		manifest := Manifest{
			SenderID:   cfg.DeviceID,
			SenderName: cfg.DeviceName,
			Items:      make([]SendItem, 0, len(files)),
		}
		for i, header := range files {
			rel := cleanBrowserRelativePath(header)
			manifest.Items = append(manifest.Items, SendItem{
				FieldName:    fmt.Sprintf("file-%d", i),
				Name:         filepath.Base(rel),
				RelativePath: filepath.ToSlash(rel),
				Size:         header.Size,
			})
		}
		data, err := json.Marshal(manifest)
		if err != nil {
			errCh <- err
			return
		}
		if err := mp.WriteField("manifest", string(data)); err != nil {
			errCh <- err
			return
		}
		for i, header := range files {
			part, err := mp.CreateFormFile(fmt.Sprintf("file-%d", i), cleanBrowserRelativePath(header))
			if err != nil {
				errCh <- err
				return
			}
			src, err := header.Open()
			if err != nil {
				errCh <- err
				return
			}
			_, copyErr := io.Copy(part, src)
			closeErr := src.Close()
			if copyErr != nil {
				errCh <- copyErr
				return
			}
			if closeErr != nil {
				errCh <- closeErr
				return
			}
		}
		if err := mp.Close(); err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	req, err := http.NewRequest(http.MethodPost, "https://"+peer.Address+"/peer/transfer", bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("X-LunarBridge-Device-ID", cfg.DeviceID)
	req.Header.Set("X-LunarBridge-Device-Name", cfg.DeviceName)
	clientCert, err := s.store.TLSCertificate()
	if err != nil {
		return err
	}
	client := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates:          []tls.Certificate{clientCert},
				InsecureSkipVerify:    true,
				VerifyPeerCertificate: config.VerifyPeerCertificate(peer.Fingerprint),
			},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		_ = bodyReader.CloseWithError(err)
		<-errCh
		return err
	}
	defer resp.Body.Close()
	if writeErr := <-errCh; writeErr != nil {
		return writeErr
	}
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("peer returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return nil
}

func (s *Service) HandleTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	senderID := r.Header.Get("X-LunarBridge-Device-ID")
	if senderID == "" {
		http.Error(w, "missing sender id", http.StatusUnauthorized)
		return
	}
	peer, ok := s.store.Peer(senderID)
	if !ok {
		http.Error(w, "sender is not paired", http.StatusUnauthorized)
		return
	}
	if len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, "sender certificate is required", http.StatusUnauthorized)
		return
	}
	got := config.FingerprintDER(r.TLS.PeerCertificates[0].Raw)
	if !strings.EqualFold(got, peer.Fingerprint) {
		http.Error(w, "sender certificate mismatch", http.StatusUnauthorized)
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "expected multipart body", http.StatusBadRequest)
		return
	}
	var manifest Manifest
	var result Result
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if part.FormName() == "manifest" {
			data, err := io.ReadAll(io.LimitReader(part, 4<<20))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				http.Error(w, "invalid manifest", http.StatusBadRequest)
				return
			}
			break
		}
	}
	if len(manifest.Items) == 0 {
		http.Error(w, "empty manifest", http.StatusBadRequest)
		return
	}
	itemByField := map[string]SendItem{}
	for _, item := range manifest.Items {
		itemByField[item.FieldName] = item
	}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item, ok := itemByField[part.FormName()]
		if !ok {
			_, _ = io.Copy(io.Discard, part)
			continue
		}
		saved, err := s.savePart(part, item)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result.Saved++
		result.Files = append(result.Files, saved)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Service) HandleLocalSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	peerID := r.FormValue("deviceId")
	if peerID == "" {
		http.Error(w, "missing deviceId", http.StatusBadRequest)
		return
	}
	peer, ok := s.store.Peer(peerID)
	if !ok {
		http.Error(w, "device is not paired", http.StatusBadRequest)
		return
	}
	if address := r.FormValue("address"); address != "" {
		peer.Address = normalizeAddress(address)
	}
	if peer.Address == "" {
		http.Error(w, "paired device has no address", http.StatusBadRequest)
		return
	}
	files := r.MultipartForm.File["files"]
	if err := s.Send(peer, files); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "count": len(files)})
}

func (s *Service) savePart(part *multipart.Part, item SendItem) (string, error) {
	cfg := s.store.Snapshot()
	target, err := config.SafeJoin(cfg.ReceiveDir, item.RelativePath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	tmp := target + ".partial"
	dst, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(dst, hasher), part)
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", closeErr
	}
	if item.Size >= 0 && written != item.Size {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("file %s expected %s bytes, received %s", item.RelativePath, strconv.FormatInt(item.Size, 10), strconv.FormatInt(written, 10))
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	_ = hex.EncodeToString(hasher.Sum(nil))
	return target, nil
}

func cleanBrowserRelativePath(header *multipart.FileHeader) string {
	name := header.Filename
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimLeft(name, "/")
	if name == "" {
		name = "unnamed"
	}
	return name
}

func normalizeAddress(address string) string {
	address = strings.TrimSpace(address)
	address = strings.TrimPrefix(address, "https://")
	address = strings.TrimPrefix(address, "http://")
	return strings.TrimRight(address, "/")
}
