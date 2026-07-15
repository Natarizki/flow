package api

import (
	"fmt"
	"net/http"

	"github.com/Natarizki/flow/pkg/utils"
)

type wifiDirectDiscovered struct {
	DeviceName    string `json:"device_name"`
	DeviceAddress string `json:"device_address"`
	Status        int    `json:"status"`
}

type wifiDirectConnected struct {
	DeviceName        string `json:"device_name"`
	DeviceAddress     string `json:"device_address"`
	GroupOwnerAddress string `json:"group_owner_address"`
	IsGroupOwner      bool   `json:"is_group_owner"`
	WSPort            int    `json:"ws_port"`
}

func (s *Server) handleWifiDirectDiscovered(w http.ResponseWriter, r *http.Request) {
	var req wifiDirectDiscovered
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	utils.LogInfo("wifi-direct: discovered device %s (%s)", req.DeviceName, req.DeviceAddress)
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

// handleWifiDirectConnected is the important one: once the Kotlin
// bridge app has actually formed a WiFi Direct group and knows the
// group owner's IP, it reports that here — and this daemon actually
// dials it via the existing Connector, treating it exactly like any
// other discovered peer address.
func (s *Server) handleWifiDirectConnected(w http.ResponseWriter, r *http.Request) {
	var req wifiDirectConnected
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.GroupOwnerAddress == "" {
		writeError(w, http.StatusBadRequest, "group_owner_address is required")
		return
	}

	wsURL := fmt.Sprintf("ws://%s:%d/ws", req.GroupOwnerAddress, req.WSPort)
	utils.LogInfo("wifi-direct: group formed with %s, dialing %s", req.DeviceName, wsURL)

	s.wifiDirectConnect(wsURL)
	writeJSON(w, http.StatusOK, map[string]string{"status": "connecting"})
}
