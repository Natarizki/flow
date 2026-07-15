package api

import (
	"net/http"
)

// ServeDashboard bikin file server handler buat folder web/dashboard,
// dipasang di path "/" (root) biar bisa dibuka langsung dari browser.
func ServeDashboard(dashboardDir string) http.Handler {
	return http.FileServer(http.Dir(dashboardDir))
}
