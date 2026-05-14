package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	AppName              = "LunarBridge"
	DefaultUIPort        = 41230
	DefaultPeerPort      = 41231
	DefaultDiscoveryPort = 41232
)

type Config struct {
	DeviceID      string          `json:"deviceId"`
	DeviceName    string          `json:"deviceName"`
	ReceiveDir    string          `json:"receiveDir"`
	UIPort        int             `json:"uiPort"`
	PeerPort      int             `json:"peerPort"`
	DiscoveryPort int             `json:"discoveryPort"`
	PairingCode   string          `json:"pairingCode"`
	Peers         map[string]Peer `json:"peers"`
	CertPEM       string          `json:"certPem"`
	KeyPEM        string          `json:"keyPem"`
}

type Peer struct {
	DeviceID    string    `json:"deviceId"`
	DeviceName  string    `json:"deviceName"`
	Fingerprint string    `json:"fingerprint"`
	Address     string    `json:"address,omitempty"`
	LastSeen    time.Time `json:"lastSeen"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

func Load() (*Store, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "config.json")
	cfg, err := loadOrCreate(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.ReceiveDir, 0o755); err != nil {
		return nil, fmt.Errorf("create receive directory: %w", err)
	}
	store := &Store{path: path, cfg: cfg}
	return store, nil
}

func (s *Store) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneConfig(s.cfg)
}

func (s *Store) Save(fn func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneConfig(s.cfg)
	if err := fn(&next); err != nil {
		return err
	}
	normalize(&next)
	if err := os.MkdirAll(next.ReceiveDir, 0o755); err != nil {
		return fmt.Errorf("create receive directory: %w", err)
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

func (s *Store) TLSCertificate() (tls.Certificate, error) {
	cfg := s.Snapshot()
	cert, err := tls.X509KeyPair([]byte(cfg.CertPEM), []byte(cfg.KeyPEM))
	if err != nil {
		return tls.Certificate{}, err
	}
	return cert, nil
}

func (s *Store) Fingerprint() (string, error) {
	cert, err := s.TLSCertificate()
	if err != nil {
		return "", err
	}
	if len(cert.Certificate) == 0 {
		return "", errors.New("missing certificate")
	}
	return FingerprintDER(cert.Certificate[0]), nil
}

func (s *Store) UpsertPeer(peer Peer) error {
	return s.Save(func(cfg *Config) error {
		if cfg.Peers == nil {
			cfg.Peers = map[string]Peer{}
		}
		if peer.LastSeen.IsZero() {
			peer.LastSeen = time.Now()
		}
		cfg.Peers[peer.DeviceID] = peer
		return nil
	})
}

func (s *Store) RemovePeer(deviceID string) error {
	return s.Save(func(cfg *Config) error {
		delete(cfg.Peers, deviceID)
		return nil
	})
}

func (s *Store) Peer(deviceID string) (Peer, bool) {
	cfg := s.Snapshot()
	peer, ok := cfg.Peers[deviceID]
	return peer, ok
}

func FingerprintDER(der []byte) string {
	sum := sha256.Sum256(der)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func VerifyPeerCertificate(wantFingerprint string) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("peer did not present a certificate")
		}
		got := FingerprintDER(rawCerts[0])
		if !strings.EqualFold(got, wantFingerprint) {
			return fmt.Errorf("peer certificate fingerprint mismatch")
		}
		return nil
	}
}

func ConfigDir() (string, error) {
	return configDir()
}

func DefaultReceiveDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", AppName)
	}
	return filepath.Join(home, "Downloads", AppName)
}

func SafeJoin(base, rel string) (string, error) {
	if rel == "" {
		return "", errors.New("empty relative path")
	}
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "\\") {
		return "", fmt.Errorf("unsafe relative path %q", rel)
	}
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if filepath.VolumeName(cleanRel) != "" || filepath.IsAbs(cleanRel) || cleanRel == "." || strings.HasPrefix(cleanRel, ".."+string(os.PathSeparator)) || cleanRel == ".." {
		return "", fmt.Errorf("unsafe relative path %q", rel)
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	target := filepath.Join(baseAbs, cleanRel)
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	relToBase, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return "", err
	}
	if relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(os.PathSeparator)) || filepath.IsAbs(relToBase) {
		return "", fmt.Errorf("unsafe relative path %q", rel)
	}
	return targetAbs, nil
}

func loadOrCreate(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	changed, err := fillDefaults(&cfg)
	if err != nil {
		return Config{}, err
	}
	if changed || errors.Is(err, os.ErrNotExist) || len(data) == 0 {
		normalize(&cfg)
		out, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return Config{}, err
		}
		if err := os.WriteFile(path, out, 0o600); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func fillDefaults(cfg *Config) (bool, error) {
	changed := false
	if cfg.DeviceID == "" {
		id, err := randomHex(16)
		if err != nil {
			return false, err
		}
		cfg.DeviceID = id
		changed = true
	}
	if cfg.DeviceName == "" {
		host, _ := os.Hostname()
		if host == "" {
			host = AppName
		}
		cfg.DeviceName = host
		changed = true
	}
	if cfg.ReceiveDir == "" {
		cfg.ReceiveDir = DefaultReceiveDir()
		changed = true
	}
	if cfg.UIPort == 0 {
		cfg.UIPort = DefaultUIPort
		changed = true
	}
	if cfg.PeerPort == 0 {
		cfg.PeerPort = DefaultPeerPort
		changed = true
	}
	if cfg.DiscoveryPort == 0 {
		cfg.DiscoveryPort = DefaultDiscoveryPort
		changed = true
	}
	if cfg.PairingCode == "" {
		code, err := randomDigits(6)
		if err != nil {
			return false, err
		}
		cfg.PairingCode = code
		changed = true
	}
	if cfg.Peers == nil {
		cfg.Peers = map[string]Peer{}
		changed = true
	}
	if cfg.CertPEM == "" || cfg.KeyPEM == "" {
		certPEM, keyPEM, err := generateCertificate(cfg.DeviceName)
		if err != nil {
			return false, err
		}
		cfg.CertPEM = certPEM
		cfg.KeyPEM = keyPEM
		changed = true
	}
	return changed, nil
}

func normalize(cfg *Config) {
	if cfg.Peers == nil {
		cfg.Peers = map[string]Peer{}
	}
	cfg.ReceiveDir = filepath.Clean(cfg.ReceiveDir)
}

func cloneConfig(cfg Config) Config {
	next := cfg
	next.Peers = map[string]Peer{}
	for k, v := range cfg.Peers {
		next.Peers[k] = v
	}
	return next
}

func configDir() (string, error) {
	if override := os.Getenv("LUNARBRIDGE_CONFIG_DIR"); override != "" {
		return filepath.Clean(override), nil
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, AppName), nil
		}
	}
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", AppName), nil
	}
	userDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userDir, AppName), nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomDigits(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = '0' + (b % 10)
	}
	return string(out), nil
}

func generateCertificate(commonName string) (string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return "", "", err
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return string(certPEM), string(keyPEM), nil
}
