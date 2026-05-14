package pairing

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"lunarbridge/internal/config"
)

type Service struct {
	store *config.Store
}

type Identity struct {
	DeviceID    string `json:"deviceId"`
	DeviceName  string `json:"deviceName"`
	Fingerprint string `json:"fingerprint"`
	PeerPort    int    `json:"peerPort"`
}

type Request struct {
	Code        string   `json:"code"`
	Address     string   `json:"address,omitempty"`
	DeviceID    string   `json:"deviceId"`
	DeviceName  string   `json:"deviceName"`
	Fingerprint string   `json:"fingerprint"`
	PeerPort    int      `json:"peerPort"`
	KnownAddrs  []string `json:"knownAddrs,omitempty"`
}

type Response struct {
	OK      bool     `json:"ok"`
	Message string   `json:"message,omitempty"`
	Device  Identity `json:"device"`
}

func New(store *config.Store) *Service {
	return &Service{store: store}
}

func (s *Service) LocalIdentity() (Identity, error) {
	cfg := s.store.Snapshot()
	fingerprint, err := s.store.Fingerprint()
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		DeviceID:    cfg.DeviceID,
		DeviceName:  cfg.DeviceName,
		Fingerprint: fingerprint,
		PeerPort:    cfg.PeerPort,
	}, nil
}

func (s *Service) Pair(address, code string) (config.Peer, error) {
	address = normalizeAddress(address)
	local, err := s.LocalIdentity()
	if err != nil {
		return config.Peer{}, err
	}
	req := Request{
		Code:        strings.TrimSpace(code),
		DeviceID:    local.DeviceID,
		DeviceName:  local.DeviceName,
		Fingerprint: local.Fingerprint,
		PeerPort:    local.PeerPort,
		KnownAddrs:  localAddresses(local.PeerPort),
	}
	var challenge Response
	if err := postJSON(insecureClient(), "https://"+address+"/peer/pair/challenge", req, &challenge); err != nil {
		return config.Peer{}, err
	}
	if !challenge.OK {
		return config.Peer{}, fmt.Errorf("%s", challenge.Message)
	}
	peer := config.Peer{
		DeviceID:    challenge.Device.DeviceID,
		DeviceName:  challenge.Device.DeviceName,
		Fingerprint: challenge.Device.Fingerprint,
		Address:     address,
		LastSeen:    time.Now(),
	}
	if err := s.store.UpsertPeer(peer); err != nil {
		return config.Peer{}, err
	}

	var confirm Response
	client := pinnedClient(peer.Fingerprint)
	if err := postJSON(client, "https://"+address+"/peer/pair/confirm", req, &confirm); err != nil {
		return peer, nil
	}
	return peer, nil
}

func (s *Service) HandleChallenge(w http.ResponseWriter, r *http.Request) {
	s.handlePair(w, r)
}

func (s *Service) HandleConfirm(w http.ResponseWriter, r *http.Request) {
	s.handlePair(w, r)
}

func (s *Service) handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	cfg := s.store.Snapshot()
	if strings.TrimSpace(req.Code) != cfg.PairingCode {
		writeJSON(w, Response{OK: false, Message: "pairing code is incorrect"})
		return
	}
	if req.DeviceID == "" || req.DeviceName == "" || req.Fingerprint == "" {
		writeJSON(w, Response{OK: false, Message: "pairing request is missing identity"})
		return
	}
	remoteAddr := peerAddressFromRequest(r, req.PeerPort)
	peer := config.Peer{
		DeviceID:    req.DeviceID,
		DeviceName:  req.DeviceName,
		Fingerprint: strings.ToUpper(req.Fingerprint),
		Address:     remoteAddr,
		LastSeen:    time.Now(),
	}
	_ = s.store.UpsertPeer(peer)
	identity, err := s.LocalIdentity()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, Response{OK: true, Device: identity})
}

func postJSON(client *http.Client, url string, in any, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("peer returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func insecureClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func pinnedClient(fingerprint string) *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify:    true,
				VerifyPeerCertificate: config.VerifyPeerCertificate(fingerprint),
			},
		},
	}
}

func peerAddressFromRequest(r *http.Request, peerPort int) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if peerPort <= 0 {
		peerPort = config.DefaultPeerPort
	}
	return net.JoinHostPort(host, fmt.Sprint(peerPort))
}

func normalizeAddress(address string) string {
	address = strings.TrimSpace(address)
	if strings.HasPrefix(address, "http://") {
		address = strings.TrimPrefix(address, "http://")
	}
	if strings.HasPrefix(address, "https://") {
		address = strings.TrimPrefix(address, "https://")
	}
	return strings.TrimRight(address, "/")
}

func localAddresses(peerPort int) []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip4 := ip.To4(); ip4 != nil {
				out = append(out, net.JoinHostPort(ip4.String(), fmt.Sprint(peerPort)))
			}
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
