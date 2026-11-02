package swu

import (
	"encoding/binary"
	"errors"
	"net"

	"github.com/wstgpt/swu-go/pkg/ipsec"
	"github.com/wstgpt/swu-go/pkg/logger"
)

const netstackInnerQueueDepth = 256

func (s *Session) initNetstackDataplane() {
	if s.innerTx != nil {
		return
	}
	s.innerTx = make(chan []byte, netstackInnerQueueDepth)
	s.innerRx = make(chan []byte, netstackInnerQueueDepth)
	s.innerClosed = make(chan struct{})
}

func (s *Session) closeNetstackDataplane() {
	if s.innerClosed == nil {
		return
	}
	select {
	case <-s.innerClosed:
	default:
		close(s.innerClosed)
	}
	if s.innerRx != nil {
		close(s.innerRx)
	}
}

// SendInnerPacket injects an inner IPv4/IPv6 packet into the userspace ESP dataplane.
func (s *Session) SendInnerPacket(packet []byte) error {
	if len(packet) == 0 {
		return nil
	}
	if s.innerTx == nil {
		return errors.New("netstack dataplane not initialized")
	}
	cp := append([]byte(nil), packet...)
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-s.innerClosed:
		return errors.New("netstack dataplane closed")
	case s.innerTx <- cp:
		return nil
	}
}

// InnerPackets returns the channel of inbound inner IP packets decrypted from ESP.
func (s *Session) InnerPackets() <-chan []byte {
	if s.innerRx == nil {
		ch := make(chan []byte)
		close(ch)
		return ch
	}
	return s.innerRx
}

func (s *Session) startNetstackDataPlaneLoop() {
	s.Logger.Info("ESP netstack 数据平面循环启动")

	go func() {
		var txCount, espSendCount, saDropCount uint64
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-s.innerClosed:
				return
			case packet, ok := <-s.innerTx:
				if !ok {
					return
				}
				if len(packet) == 0 {
					continue
				}
				txCount++

				var dstIP string
				var proto uint8
				if len(packet) > 0 {
					ver := packet[0] >> 4
					if ver == 4 && len(packet) >= 20 {
						dstIP = net.IP(packet[16:20]).String()
						proto = packet[9]
					} else if ver == 6 && len(packet) >= 40 {
						dstIP = net.IP(packet[24:40]).String()
						proto = packet[6]
					}
				}

				saOut := s.selectOutgoingSA(packet)
				if saOut == nil {
					saDropCount++
					if saDropCount <= 5 || saDropCount%100 == 0 {
						s.Logger.Warn("netstack ESP 出站 SA 为空，丢弃数据包",
							logger.Uint64("dropCount", saDropCount),
							logger.String("dstIP", dstIP),
							logger.Int("proto", int(proto)),
							logger.Int("len", len(packet)))
					}
					continue
				}

				espPacket, err := ipsec.Encapsulate(packet, saOut)
				if err != nil {
					s.Logger.Warn("netstack ESP 封装错误", logger.Err(err), logger.String("dstIP", dstIP))
					continue
				}
				if err := s.socket.SendESP(espPacket); err != nil {
					s.Logger.Warn("netstack ESP 发送失败", logger.Err(err), logger.String("dstIP", dstIP))
					continue
				}
				espSendCount++
			}
		}
	}()

	go func() {
		var espRecvCount, rxCount uint64
		for espData := range s.socket.ESPPackets() {
			select {
			case <-s.ctx.Done():
				return
			case <-s.innerClosed:
				return
			default:
			}

			espRecvCount++

			var spi uint32
			if len(espData) >= 4 {
				spi = binary.BigEndian.Uint32(espData[0:4])
			}

			if espRecvCount <= 30 || espRecvCount%100 == 0 {
				s.Logger.Info("ESP 入站包", logger.Uint64("count", espRecvCount), logger.Uint32("spi", spi), logger.Int("len", len(espData)))
			}

			sa := s.ChildSAIn
			if len(espData) >= 4 && s.ChildSAsIn != nil {
				if hit, ok := s.ChildSAsIn[spi]; ok {
					sa = hit
				}
			}
			if sa == nil {
				s.Logger.Warn("netstack ESP 入站 SA 为空，丢弃数据包", logger.Uint32("spi", spi), logger.Int("len", len(espData)))
				continue
			}

			packet, err := ipsec.Decapsulate(espData, sa)
			if err != nil {
				s.Logger.Warn("netstack ESP 解封装错误", logger.Err(err), logger.Uint32("spi", spi), logger.Int("len", len(espData)))
				continue
			}

			if espRecvCount <= 30 || espRecvCount%100 == 0 {
				s.Logger.Info("ESP 解密成功→innerRx", logger.Uint64("rx", rxCount+1), logger.Int("innerLen", len(packet)))
			}

			cp := append([]byte(nil), packet...)
			select {
			case <-s.ctx.Done():
				return
			case <-s.innerClosed:
				return
			case s.innerRx <- cp:
				rxCount++
			default:
				s.Logger.Warn("netstack 入站队列已满，丢弃数据包", logger.Int("len", len(packet)))
			}
		}
	}()
}
