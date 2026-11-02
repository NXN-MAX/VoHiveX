package ipsec

// Darwin has no Linux MSG_ERRQUEUE. IKE DPD still monitors tunnel liveness.
func (s *SocketManager) startErrorListener() {}
