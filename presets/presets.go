package presets

import (
	"fmt"
	"net/http"

	"github.com/bluegradienthorizon/proxytoolbox/worker"
)

const (
	Google204         = "https://www.google.com/generate_204"
	GStatic204        = "https://www.gstatic.com/generate_204"
	PlayGoogleAPIs204 = "https://play.googleapis.com/generate_204"
	CPCloudflare204   = "https://cp.cloudflare.com/generate_204"
)

var CloudflareProvider = worker.SpeedTestProvider{
	GetURL: func(mode worker.SpeedTestMode, targetBytes int64) string {
		const (
			Down = "https://speed.cloudflare.com/__down"
			Up   = "https://speed.cloudflare.com/__up"
		)
		var u string
		switch mode {
		case worker.SpeedTestModeDownload:
			u = fmt.Sprintf("%s?bytes=%d", Down, targetBytes)
		case worker.SpeedTestModeUpload:
			u = Up
		}
		return u
	},
	ModifyRequest: func(req *http.Request, mode worker.SpeedTestMode, targetBytes int64) {
		if mode == worker.SpeedTestModeUpload {
			req.ContentLength = targetBytes
		}
	},
}
