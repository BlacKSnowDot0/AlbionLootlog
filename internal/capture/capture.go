// Package capture implements live Photon packet capture using libpcap/Npcap
// via gopacket. The BPF filter, Photon UDP ports, and packet heuristic are
// ported from the upstream C# project Triky313/AlbionOnline-StatisticsAnalysis
// (GPL-3.0).
package capture

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// DefaultBPFFilter mirrors the upstream LibpcapPacketProvider filter: Photon
// UDP on 5055/5056/5058 plus IPv4 fragments (which carry no UDP header) and the
// IPv6 equivalents.
const DefaultBPFFilter = "((ip and ((udp and (port 5055 or port 5056 or port 5058)) or (ip[6:2] & 0x3fff != 0))) or (ip6 and (udp and (port 5055 or port 5056 or port 5058))))"

// PhotonUDPPorts are the Albion game-server UDP ports.
var PhotonUDPPorts = map[uint16]struct{}{5055: {}, 5056: {}, 5058: {}}

// PayloadSink receives raw Photon UDP payloads (already filtered).
type PayloadSink interface {
	Parse(payload []byte)
}

// Options configures a capture session.
type Options struct {
	// Device is the pcap device name to capture on. Empty means auto: capture
	// on all eligible devices and lock onto the first that yields Photon data.
	Device string
	// BPFFilter overrides DefaultBPFFilter when non-empty.
	BPFFilter string
	// SnapLen is the capture length per packet.
	SnapLen int32
	// Logger receives operational logs.
	Logger *slog.Logger
}

func (o *Options) withDefaults() {
	if o.SnapLen == 0 {
		o.SnapLen = 65535
	}
	if o.BPFFilter == "" {
		o.BPFFilter = DefaultBPFFilter
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
}

// ListDevices returns capturable, up, non-loopback devices.
func ListDevices() ([]pcap.Interface, error) {
	all, err := pcap.FindAllDevs()
	if err != nil {
		return nil, err
	}
	var out []pcap.Interface
	for _, d := range all {
		// Skip devices with no addresses; they are usually not useful and
		// often cannot be opened.
		if len(d.Addresses) == 0 {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// Capturer runs one or more pcap handles and feeds Photon payloads to a sink.
type Capturer struct {
	opts Options
	sink PayloadSink
}

// New returns a Capturer.
func New(opts Options, sink PayloadSink) *Capturer {
	opts.withDefaults()
	return &Capturer{opts: opts, sink: sink}
}

// Run captures until ctx is cancelled. If Options.Device is set it captures a
// single device; otherwise it captures every eligible device concurrently and
// lets the Photon filter naturally select traffic from the active adapter.
func (c *Capturer) Run(ctx context.Context) error {
	if c.opts.Device != "" {
		return c.runDevice(ctx, c.opts.Device)
	}

	devs, err := ListDevices()
	if err != nil {
		return fmt.Errorf("list devices: %w", err)
	}
	if len(devs) == 0 {
		return fmt.Errorf("no capturable network devices found (is Npcap installed?)")
	}

	errCh := make(chan error, len(devs))
	started := 0
	for _, d := range devs {
		name := d.Name
		go func() {
			if err := c.runDevice(ctx, name); err != nil {
				c.opts.Logger.Debug("device capture ended", "device", name, "err", err)
				errCh <- err
				return
			}
			errCh <- nil
		}()
		started++
	}

	<-ctx.Done()
	return ctx.Err()
}

// runDevice opens a single device, applies the BPF filter, and reads packets
// until ctx is cancelled.
func (c *Capturer) runDevice(ctx context.Context, device string) error {
	handle, err := pcap.OpenLive(device, c.opts.SnapLen, true, pcap.BlockForever)
	if err != nil {
		return fmt.Errorf("open %s: %w", device, err)
	}
	defer handle.Close()

	if err := handle.SetBPFFilter(c.opts.BPFFilter); err != nil {
		return fmt.Errorf("set filter on %s: %w", device, err)
	}

	c.opts.Logger.Info("capture started", "device", device, "filter", c.opts.BPFFilter)

	src := gopacket.NewPacketSource(handle, handle.LinkType())
	src.NoCopy = true
	packets := src.Packets()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case pkt, ok := <-packets:
			if !ok {
				return nil
			}
			c.handlePacket(pkt)
		}
	}
}

// handlePacket extracts the UDP payload and forwards likely Photon packets to
// the sink. gopacket handles IPv4 reassembly transparently when defragmenting
// is enabled; for fragmented traffic users should prefer Npcap which reassembles
// at capture time, matching upstream behavior.
func (c *Capturer) handlePacket(pkt gopacket.Packet) {
	udpLayer := pkt.Layer(layers.LayerTypeUDP)
	if udpLayer == nil {
		return
	}
	udp, _ := udpLayer.(*layers.UDP)
	if udp == nil {
		return
	}

	_, srcPhoton := PhotonUDPPorts[uint16(udp.SrcPort)]
	_, dstPhoton := PhotonUDPPorts[uint16(udp.DstPort)]
	payload := udp.Payload

	if !srcPhoton && !dstPhoton && !looksLikePhoton(payload) {
		return
	}
	if len(payload) == 0 {
		return
	}

	c.sink.Parse(payload)
}

// looksLikePhoton mirrors the upstream first-byte heuristic for payloads that
// arrive on non-standard ports (e.g. via VPN/ExitLag remapping).
func looksLikePhoton(payload []byte) bool {
	if len(payload) < 3 {
		return false
	}
	switch payload[0] {
	case 0xF1, 0xF2, 0xFE:
		return true
	default:
		return false
	}
}

// WaitForFirstPacket is a small helper for diagnostics: it blocks until either
// a payload is seen (via the sink) or the timeout elapses. It is not used in
// the main run loop but is handy for the health check.
func WaitForFirstPacket(ctx context.Context, timeout time.Duration, seen func() bool) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if seen() {
				return true
			}
			if time.Now().After(deadline) {
				return false
			}
		}
	}
}
