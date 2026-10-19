package updatecontract

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLivePreviewManifestThroughPlatformHTTPS(t *testing.T) {
	if os.Getenv("MEOW_LIVE_MANIFEST_TEST") != "1" {
		t.Skip("set MEOW_LIVE_MANIFEST_TEST=1 to run the live manifest acceptance test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result := Check(ctx, "https://shizukuocean.github.io/meow-releases/channels/preview/manifest-v1.json", "dev-helper", "0.3.0-p1", "preview")
	if result.Status != StatusUpdateAvailable || result.Release == nil || result.Release.ProductID != "dev-helper" {
		t.Fatalf("unexpected live Helper result: %+v", result)
	}
}
