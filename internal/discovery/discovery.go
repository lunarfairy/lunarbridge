package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"lunarbridge/internal/config"
)

const protocol = "lunarbridge.v1"

type Device struct {
	DeviceID   string    `json:"deviceId"`
	DeviceName string    `json:"deviceName"`
	Address    string    `json:"address"`
	PeerPort   int       `json:"peerPort"`
	LastSeen   time.Time `json:"lastSeen"`
	Manual     bool      `json:"manual"`
	Paired     bool      `json:"paired"`
}

type Service struct {
	store *config.Store

	mu      sync.RWMutex
	devices map[string]Device
}

type packet struct {
	Protocol   string `json:"protocol"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	PeerPort   int    `json:"peerPort"`
}

func New(store *config.Store) *Service {
	return &Service{
		store:   store,
		devices: map[string]Device{},
	}
}

func (s *Service) Run(ctx context.Context) {
	go s.listen(ctx)
	go s.broadcast(ctx)
}

func (s *Service) Snapshot() []Device {
	cfg := s.store.Snapshot()
	s.mu.RLock()
	defer s.mu.RUnlock()
	devices := make([]Device, 0, len(s.devices))
	cutoff := time.Now().Add(-45 * time.Second)
	for _, device := range s.devices {
		if !device.Manual && device.LastSeen.Before(cutoff) {
			continue
		}
		_, device.Paired = cfg.Peers[device.DeviceID]
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Paired != devices[j].Paired {
			return devices[i].Paired
		}
		return devices[i].DeviceName < devices[j].DeviceName
	})
	return devices
}

func (s *Service) AddManual(address string) (Device, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return Device{}, fmt.Errorf("address must look like IP:port")
	}
	parsedPort, err := net.LookupPort("tcp", port)
	if err != nil {
		return Device{}, err
	}
	if ip := net.ParseIP(host); ip == nil {
		return Device{}, fmt.Errorf("manual address must use an IP address")
	}
	device := Device{
		DeviceID:   "manual-" + address,
		DeviceName: "Manual " + address,
		Address:    net.JoinHostPort(host, port),
		PeerPort:   parsedPort,
		LastSeen:   time.Now(),
		Manual:     true,
	}
	s.mu.Lock()
	s.devices[device.DeviceID] = device
	s.mu.Unlock()
	return device, nil
}

func (s *Service) Device(deviceID string) (Device, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	device, ok := s.devices[deviceID]
	return device, ok
}

func (s *Service) UpsertPeerAddress(deviceID, deviceName, address string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	device := s.devices[deviceID]
	device.DeviceID = deviceID
	if deviceName != "" {
		device.DeviceName = deviceName
	} else if device.DeviceName == "" {
		device.DeviceName = deviceID
	}
	device.Address = address
	if _, port, err := net.SplitHostPort(address); err == nil {
		device.PeerPort, _ = net.LookupPort("tcp", port)
	}
	device.LastSeen = time.Now()
	s.devices[deviceID] = device
}

func (s *Service) listen(ctx context.Context) {
	cfg := s.store.Snapshot()
	addr := &net.UDPAddr{IP: net.IPv4zero, Port: cfg.DiscoveryPort}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	buf := make([]byte, 4096)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		var pkt packet
		if err := json.Unmarshal(buf[:n], &pkt); err != nil {
			continue
		}
		cfg := s.store.Snapshot()
		if pkt.Protocol != protocol || pkt.DeviceID == "" || pkt.DeviceID == cfg.DeviceID {
			continue
		}
		device := Device{
			DeviceID:   pkt.DeviceID,
			DeviceName: pkt.DeviceName,
			Address:    net.JoinHostPort(remote.IP.String(), fmt.Sprint(pkt.PeerPort)),
			PeerPort:   pkt.PeerPort,
			LastSeen:   time.Now(),
		}
		s.mu.Lock()
		s.devices[pkt.DeviceID] = device
		s.mu.Unlock()
	}
}

func (s *Service) broadcast(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		s.sendBroadcast()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) sendBroadcast() {
	cfg := s.store.Snapshot()
	payload, err := json.Marshal(packet{
		Protocol:   protocol,
		DeviceID:   cfg.DeviceID,
		DeviceName: cfg.DeviceName,
		PeerPort:   cfg.PeerPort,
	})
	if err != nil {
		return
	}
	addr := &net.UDPAddr{IP: net.IPv4bcast, Port: cfg.DiscoveryPort}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, _ = conn.Write(payload)
}
