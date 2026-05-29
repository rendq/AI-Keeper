package router

import (
	"testing"
)

func TestRegionAwareRouting(t *testing.T) {
	rr := NewRegionRouter()

	endpoints := []ModelEndpointRegion{
		{Endpoint: "https://cn.api.example.com", Region: "cn-north", Continent: "asia"},
		{Endpoint: "https://eu.api.example.com", Region: "eu-west", Continent: "europe"},
		{Endpoint: "https://us.api.example.com", Region: "us-east", Continent: "americas"},
		{Endpoint: "https://jp.api.example.com", Region: "ap-northeast", Continent: "asia"},
	}

	t.Run("exact region match preferred", func(t *testing.T) {
		userRegion := CountryToRegion("CN") // cn-north
		ep, err := rr.RouteByRegion(userRegion, endpoints, RegionRoutingConfig{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ep.Region != "cn-north" {
			t.Errorf("expected cn-north, got %s", ep.Region)
		}
	})

	t.Run("falls back to same continent", func(t *testing.T) {
		// ap-south has no exact match, but continent is "asia"
		ep, err := rr.RouteByRegion("ap-south", endpoints, RegionRoutingConfig{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should pick an asia endpoint (cn-north or ap-northeast)
		if ep.Continent != "asia" {
			t.Errorf("expected asia continent, got %s", ep.Continent)
		}
	})

	t.Run("denies cross-border when forbidden", func(t *testing.T) {
		// Only EU endpoints available, user is in a region with no match
		euOnly := []ModelEndpointRegion{
			{Endpoint: "https://eu.api.example.com", Region: "eu-west", Continent: "europe"},
		}
		config := RegionRoutingConfig{ForbidCrossBorder: true}
		_, err := rr.RouteByRegion("cn-north", euOnly, config)
		if err != ErrCrossBorderDenied {
			t.Errorf("expected ErrCrossBorderDenied, got %v", err)
		}
	})

	t.Run("allows any endpoint when forbidCrossBorder is false", func(t *testing.T) {
		euOnly := []ModelEndpointRegion{
			{Endpoint: "https://eu.api.example.com", Region: "eu-west", Continent: "europe"},
		}
		config := RegionRoutingConfig{ForbidCrossBorder: false}
		ep, err := rr.RouteByRegion("cn-north", euOnly, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ep.Endpoint != "https://eu.api.example.com" {
			t.Errorf("expected eu endpoint, got %s", ep.Endpoint)
		}
	})

	t.Run("no endpoints returns error", func(t *testing.T) {
		_, err := rr.RouteByRegion("cn-north", nil, RegionRoutingConfig{})
		if err != ErrNoEndpoints {
			t.Errorf("expected ErrNoEndpoints, got %v", err)
		}
	})
}

func TestCountryToRegion(t *testing.T) {
	tests := []struct {
		country string
		want    string
	}{
		{"CN", "cn-north"},
		{"US", "us-east"},
		{"DE", "eu-west"},
		{"XX", "us-east"}, // unknown defaults to us-east
	}
	for _, tt := range tests {
		got := CountryToRegion(tt.country)
		if got != tt.want {
			t.Errorf("CountryToRegion(%q) = %q, want %q", tt.country, got, tt.want)
		}
	}
}

func TestRegionToContinent(t *testing.T) {
	tests := []struct {
		region string
		want   string
	}{
		{"cn-north", "asia"},
		{"us-east", "americas"},
		{"eu-west", "europe"},
		{"unknown-region", "unknown"},
	}
	for _, tt := range tests {
		got := RegionToContinent(tt.region)
		if got != tt.want {
			t.Errorf("RegionToContinent(%q) = %q, want %q", tt.region, got, tt.want)
		}
	}
}
