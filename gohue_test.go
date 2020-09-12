package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDiscoverHueBridge(t *testing.T) {
	var want HueBridges
	hue_api_key := "12345678"
	influx_db_address := "192.168.100.100"
	hueDiscoveryUrl := "https://discovery.meethue.com/"
	if got := discoverHueBridges(hue_api_key, influx_db_address, hueDiscoveryUrl); cmp.Equal(got, want) {
		t.Errorf("Hello() = %q, want %q", got, want)
	}
}
