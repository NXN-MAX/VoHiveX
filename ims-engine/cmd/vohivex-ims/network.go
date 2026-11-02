package main

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	ipheader "gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

type packetTunnel interface {
	SendInnerPacket([]byte) error
	InnerPackets() <-chan []byte
}
type innerNetwork struct {
	s        *stack.Stack
	link     *channel.Endpoint
	ip       net.IP
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	security atomic.Pointer[imsSecurity]
}

func newInnerNetwork(ctx context.Context, ip net.IP, t packetTunnel) (*innerNetwork, error) {
	ctx, cancel := context.WithCancel(ctx)
	n := &innerNetwork{ip: ip, cancel: cancel, link: channel.New(256, 1358, "")}
	n.s = stack.New(stack.Options{NetworkProtocols: []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol}, TransportProtocols: []stack.TransportProtocolFactory{udp.NewProtocol, tcp.NewProtocol, icmp.NewProtocol4, icmp.NewProtocol6}})
	if e := n.s.CreateNIC(1, n.link); e != nil {
		cancel()
		return nil, fmt.Errorf("netstack NIC: %s", e)
	}
	proto, addr := netAddress(ip)
	if e := n.s.AddProtocolAddress(1, tcpip.ProtocolAddress{Protocol: proto, AddressWithPrefix: addr.WithPrefix()}, stack.AddressProperties{}); e != nil {
		n.close()
		return nil, fmt.Errorf("netstack address: %s", e)
	}
	n.s.SetRouteTable([]tcpip.Route{{Destination: ipheader.IPv4EmptySubnet, NIC: 1}, {Destination: ipheader.IPv6EmptySubnet, NIC: 1}})
	n.wg.Add(2)
	go func() {
		defer n.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-t.InnerPackets():
				if !ok {
					return
				}
				if sec := n.security.Load(); sec != nil {
					var e error
					data, e = sec.open(data)
					if e != nil {
						continue
					}
				}
				if len(data) < 1 {
					continue
				}
				var proto tcpip.NetworkProtocolNumber
				switch data[0] >> 4 {
				case 4:
					proto = ipv4.ProtocolNumber
				case 6:
					proto = ipv6.ProtocolNumber
				default:
					continue
				}
				p := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(data)})
				n.link.InjectInbound(proto, p)
				p.DecRef()
			}
		}
	}()
	go func() {
		defer n.wg.Done()
		for {
			p := n.link.ReadContext(ctx)
			if p == nil {
				return
			}
			v := p.ToView()
			data := append([]byte(nil), v.AsSlice()...)
			v.Release()
			p.DecRef()
			if sec := n.security.Load(); sec != nil {
				var e error
				data, e = sec.seal(data)
				if e != nil {
					continue
				}
			}
			if e := t.SendInnerPacket(data); e != nil {
				return
			}
		}
	}()
	return n, nil
}
func netAddress(ip net.IP) (tcpip.NetworkProtocolNumber, tcpip.Address) {
	if v := ip.To4(); v != nil {
		return ipv4.ProtocolNumber, tcpip.AddrFromSlice(v)
	}
	return ipv6.ProtocolNumber, tcpip.AddrFromSlice(ip.To16())
}
func (n *innerNetwork) listenUDP(port uint16) (net.PacketConn, error) {
	proto, ip := netAddress(n.ip)
	return gonet.DialUDP(n.s, &tcpip.FullAddress{NIC: 1, Addr: ip, Port: port}, nil, proto)
}
func (n *innerNetwork) dialTCP(ctx context.Context, port uint16, remote *net.UDPAddr) (net.PacketConn, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	proto, local := netAddress(n.ip)
	_, peer := netAddress(remote.IP)
	c, err := gonet.DialTCPWithBind(ctx, n.s,
		tcpip.FullAddress{NIC: 1, Addr: local, Port: port},
		tcpip.FullAddress{NIC: 1, Addr: peer, Port: uint16(remote.Port)}, proto)
	if err != nil {
		return nil, err
	}
	return newSIPStream(c), nil
}
func (n *innerNetwork) close() { n.cancel(); n.link.Close(); n.s.Close(); n.wg.Wait(); n.s.Wait() }
