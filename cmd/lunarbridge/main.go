package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"lunarbridge/internal/config"
	"lunarbridge/internal/discovery"
	"lunarbridge/internal/pairing"
	"lunarbridge/internal/transfer"
	"lunarbridge/web"
)

type app struct {
	store     *config.Store
	discovery *discovery.Service
	pairing   *pairing.Service
	transfer  *transfer.Service
}

func main() {
	var configDir string
	var uiPort int
	var peerPort int
	var discoveryPort int
	var noBrowser bool
	flag.StringVar(&configDir, "config-dir", "", "override LunarBridge config directory")
	flag.IntVar(&uiPort, "ui-port", 0, "override local web UI port")
	flag.IntVar(&peerPort, "peer-port", 0, "override peer HTTPS port")
	flag.IntVar(&discoveryPort, "discovery-port", 0, "override UDP discovery port")
	flag.BoolVar(&noBrowser, "no-browser", false, "do not open the browser on startup")
	flag.Parse()

	if configDir != "" {
		_ = os.Setenv("LUNARBRIDGE_CONFIG_DIR", configDir)
	}
	store, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if uiPort != 0 || peerPort != 0 || discoveryPort != 0 {
		if err := store.Save(func(cfg *config.Config) error {
			if uiPort != 0 {
				cfg.UIPort = uiPort
			}
			if peerPort != 0 {
				cfg.PeerPort = peerPort
			}
			if discoveryPort != 0 {
				cfg.DiscoveryPort = discoveryPort
			}
			return nil
		}); err != nil {
			log.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := &app{
		store: store,
	}
	a.discovery = discovery.New(store)
	a.pairing = pairing.New(store)
	a.transfer = transfer.New(store)

	a.discovery.Run(ctx)
	if err := a.startPeerServer(ctx); err != nil {
		log.Fatal(err)
	}
	uiURL, err := a.startUIServer(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("LunarBridge is running: %s\n", uiURL)
	if !noBrowser {
		go openBrowser(uiURL)
	}
	select {}
}

func (a *app) startPeerServer(ctx context.Context) error {
	cfg := a.store.Snapshot()
	cert, err := a.store.TLSCertificate()
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/peer/pair/challenge", a.pairing.HandleChallenge)
	mux.HandleFunc("/peer/pair/confirm", a.pairing.HandleConfirm)
	mux.HandleFunc("/peer/transfer", a.transfer.HandleTransfer)
	server := &http.Server{
		Addr:    ":" + strconv.Itoa(cfg.PeerPort),
		Handler: requestLogger(mux),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequestClientCert,
			MinVersion:   tls.VersionTLS12,
		},
	}
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		shutdown(server)
	}()
	go func() {
		if err := server.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
			log.Printf("peer server stopped: %v", err)
		}
	}()
	return nil
}

func (a *app) startUIServer(ctx context.Context) (string, error) {
	cfg := a.store.Snapshot()
	static, err := fs.Sub(web.Files, ".")
	if err != nil {
		return "", err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/settings", a.handleSettings)
	mux.HandleFunc("/api/devices", a.handleDevices)
	mux.HandleFunc("/api/manual", a.handleManual)
	mux.HandleFunc("/api/pair", a.handlePair)
	mux.HandleFunc("/api/send", a.transfer.HandleLocalSend)
	mux.Handle("/", http.FileServer(http.FS(static)))
	server := &http.Server{
		Addr:    "127.0.0.1:" + strconv.Itoa(cfg.UIPort),
		Handler: requestLogger(mux),
	}
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return "", err
	}
	go func() {
		<-ctx.Done()
		shutdown(server)
	}()
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("ui server stopped: %v", err)
		}
	}()
	return "http://" + server.Addr + "/", nil
}

func (a *app) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := a.store.Snapshot()
		fingerprint, _ := a.store.Fingerprint()
		writeJSON(w, map[string]any{
			"deviceId":      cfg.DeviceID,
			"deviceName":    cfg.DeviceName,
			"receiveDir":    cfg.ReceiveDir,
			"uiPort":        cfg.UIPort,
			"peerPort":      cfg.PeerPort,
			"discoveryPort": cfg.DiscoveryPort,
			"pairingCode":   cfg.PairingCode,
			"fingerprint":   fingerprint,
			"uiAddress":     "127.0.0.1:" + strconv.Itoa(cfg.UIPort),
			"peerAddress":   primaryLANAddress(cfg.PeerPort),
			"peers":         cfg.Peers,
		})
	case http.MethodPost:
		var req struct {
			DeviceName string `json:"deviceName"`
			ReceiveDir string `json:"receiveDir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := a.store.Save(func(cfg *config.Config) error {
			if strings.TrimSpace(req.DeviceName) != "" {
				cfg.DeviceName = strings.TrimSpace(req.DeviceName)
			}
			if strings.TrimSpace(req.ReceiveDir) != "" {
				cfg.ReceiveDir = strings.TrimSpace(req.ReceiveDir)
			}
			return nil
		}); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	devices := a.discovery.Snapshot()
	cfg := a.store.Snapshot()
	seen := map[string]bool{}
	for _, device := range devices {
		seen[device.DeviceID] = true
	}
	for _, peer := range cfg.Peers {
		if seen[peer.DeviceID] {
			continue
		}
		devices = append(devices, discovery.Device{
			DeviceID:   peer.DeviceID,
			DeviceName: peer.DeviceName,
			Address:    peer.Address,
			LastSeen:   peer.LastSeen,
			Paired:     true,
		})
	}
	writeJSON(w, devices)
}

func (a *app) handleManual(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	device, err := a.discovery.AddManual(req.Address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, device)
}

func (a *app) handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Address string `json:"address"`
		Code    string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	peer, err := a.pairing.Pair(req.Address, req.Code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.discovery.UpsertPeerAddress(peer.DeviceID, peer.DeviceName, peer.Address)
	writeJSON(w, peer)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func shutdown(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func primaryLANAddress(port int) string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1:" + strconv.Itoa(port)
	}
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
			if ip4 := ip.To4(); ip4 != nil && !ip4.IsLoopback() {
				return net.JoinHostPort(ip4.String(), strconv.Itoa(port))
			}
		}
	}
	return "127.0.0.1:" + strconv.Itoa(port)
}
