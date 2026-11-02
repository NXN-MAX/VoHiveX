package ipsec

// NetEvent 描述从错误队列收到的网络事件
type NetEvent struct {
	Type    NetEventType
	PMTU    uint32 // 如果是 PathMTU，这里会有新的 MTU
	Reason  string
	OldPort int // NAT-T 端口漂移前的旧端口
	NewPort int // NAT-T 端口漂移后的新端口
}

type NetEventType int

const (
	EventPathMTU        NetEventType = iota // 收到了 ICMP Frag Needed / Packet Too Big
	EventNetworkDown                        // 收到了 Host / Net Unreachable (用于 DPD 欺骗预测)
	EventNATPortChanged                     // NAT-T 端口漂移：远端源端口发生了变化 (RFC 3947)
)
