package config

import (
	"testing"
	"time"
)

func TestCaptureAndRateSettings(t *testing.T) {
	cfg, err := FromLookup(lookup(nil))
	if err != nil || !cfg.CaptureEnabled || cfg.SpotifyRate != 0 || cfg.CapturePlayingInterval != 0 ||
		cfg.ImportMaxBytes != DefaultImportMaxBytes {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	cfg, err = FromLookup(lookup(map[string]string{
		"SPOTIFY_RATE": "1.5", "SPOTIFY_BURST": "20", "SPOTIFY_LIVE_RESERVE": "0.25", "CAPTURE_ENABLED": "false",
		"SPOTIFY_LOOPBACK_PORT": "18080", "CAPTURE_PLAYING_INTERVAL": "10s", "CAPTURE_IDLE_INTERVAL": "2m", "CAPTURE_RECENT_INTERVAL": "30m",
		"IMPORT_MAX_BYTES": "1048576",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SpotifyRate != 1.5 || cfg.SpotifyLoopbackPort != 18080 || cfg.SpotifyBurst != 20 || cfg.SpotifyLiveReserve != 0.25 || cfg.CaptureEnabled ||
		cfg.CapturePlayingInterval != 10*time.Second || cfg.CaptureIdleInterval != 2*time.Minute ||
		cfg.CaptureRecentInterval != 30*time.Minute || cfg.ImportMaxBytes != 1<<20 {
		t.Fatalf("values: %+v", cfg)
	}
	for _, env := range []map[string]string{
		{"SPOTIFY_RATE": "fast"}, {"SPOTIFY_LOOPBACK_PORT": "70000"}, {"SPOTIFY_RATE": "0"}, {"SPOTIFY_BURST": "0"}, {"SPOTIFY_LIVE_RESERVE": "1"},
		{"CAPTURE_PLAYING_INTERVAL": "1s"}, {"CAPTURE_IDLE_INTERVAL": "soon"}, {"CAPTURE_RECENT_INTERVAL": "10s"},
		{"CAPTURE_ENABLED": "maybe"}, {"IMPORT_MAX_BYTES": "1000"}, {"IMPORT_MAX_BYTES": "2GiB"},
	} {
		if _, err := FromLookup(lookup(env)); err == nil {
			t.Errorf("%v: expected error", env)
		}
	}
}

func TestMetadataSettings(t *testing.T) {
	cfg, err := FromLookup(lookup(nil))
	if err != nil || !cfg.HydrateSpotifyLookups || !cfg.MetadataOEmbed || !cfg.MetadataDeezer || !cfg.MetadataListenBrainz || !cfg.MetadataMusicBrainz || cfg.MetadataContact != "" {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	cfg, err = FromLookup(lookup(map[string]string{"HYDRATE_SPOTIFY_LOOKUPS": "false", "METADATA_DEEZER": "false", "METADATA_MUSICBRAINZ": "0",
		"METADATA_LISTENBRAINZ": "false", "METADATA_CONTACT": " https://example.org/ "}))
	if err != nil || cfg.HydrateSpotifyLookups || !cfg.MetadataOEmbed || cfg.MetadataDeezer || cfg.MetadataListenBrainz || cfg.MetadataMusicBrainz ||
		cfg.MetadataContact != "https://example.org/" {
		t.Fatalf("values: %+v %v", cfg, err)
	}
	if _, err := FromLookup(lookup(map[string]string{"METADATA_OEMBED": "sometimes"})); err == nil {
		t.Error("expected error")
	}
}
