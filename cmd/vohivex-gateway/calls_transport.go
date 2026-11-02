package main

// Follow the device's VoWiFi policy, including while it is reconnecting.
// network_enabled controls Internet data, not cellular voice capability.
func deviceCallTransport(device map[string]any) string {
	if active, _ := device["vowifi_active"].(bool); active {
		return "vowifi"
	}
	if runtime, ok := device["vowifi_runtime"].(map[string]any); ok {
		if runtime["dataplane_mode"] == "vowifi" {
			return "vowifi"
		}
	}
	if enabled, known := device["vowifi_enabled"].(bool); known {
		if enabled {
			return "vowifi"
		}
		return "cellular"
	}
	// Missing or malformed policy is not permission to place a cellular call.
	return "unknown"
}

type callRoute struct {
	Transport       string `json:"transport"`
	DeviceTransport string `json:"device_transport"`
	Available       bool   `json:"call_control_available"`
	Detail          string `json:"call_control_detail"`
}

func resolveCallRoute(device map[string]any, nativeCalls []callHistoryItem) callRoute {
	route := callRoute{DeviceTransport: deviceCallTransport(device)}
	route.Transport = route.DeviceTransport
	if device["audio_preparing"] == true {
		route.Detail = "正在准备或恢复模组音频接口"
		return route
	}
	// All persisted calls in the current native provider belong to cellular.
	// Finish/observe that same session even when policy changes mid-call.
	if len(nativeCalls) > 0 {
		route.Transport = "cellular"
	}
	switch route.Transport {
	case "cellular":
		route.Available = true
		route.Detail = "跟随设备模式，当前使用蜂窝电话通道"
		if route.DeviceTransport != route.Transport {
			route.Detail = "当前通话保持原蜂窝通道；通话结束后跟随设备模式"
		}
	case "vowifi":
		route.Detail = "设备使用 VoWiFi；当前网关尚未接入 VoWiFi 电话控制，不会改走蜂窝通道"
	default:
		route.Detail = "无法确认设备电话模式，未选择拨号通道"
	}
	return route
}

func (route callRoute) writeFields(result map[string]any) {
	result["transport"] = route.Transport
	result["device_transport"] = route.DeviceTransport
	result["call_control_available"] = route.Available
	result["call_control_detail"] = route.Detail
	result["vowifi_call_supported"] = false
}
