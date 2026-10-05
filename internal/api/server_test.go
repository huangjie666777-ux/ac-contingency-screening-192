package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const validRequest = `{
  "base_mva": 100,
  "buses": [
    {"id":"B1","type":"slack","v_min_pu":0.9,"v_max_pu":1.1,"p_mw":0,"q_mvar":0,"voltage_pu":1.0,"angle_deg":0},
    {"id":"B2","type":"pq","v_min_pu":0.9,"v_max_pu":1.1,"p_mw":-50,"q_mvar":-10,"voltage_pu":0,"angle_deg":0}
  ],
  "lines": [
    {"id":"L1","from_bus":"B1","to_bus":"B2","r_pu":0.01,"x_pu":0.05,"b_pu":0.01,"capacity_mva":100}
  ]
}`

func TestScreenEndpoint(t *testing.T) {
	srv := httptest.NewServer(NewServer().Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/screen", "application/json", strings.NewReader(validRequest))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var report map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	base := report["base_case"].(map[string]any)
	if base["status"] != "safe" {
		t.Fatalf("base case = %v", base["status"])
	}
	cont := report["contingencies"].([]any)
	if len(cont) != 1 {
		t.Fatalf("contingencies len = %d", len(cont))
	}
	if cont[0].(map[string]any)["status"] != "deenergized" {
		t.Fatalf("outage = %v", cont[0])
	}
}

func TestRejectsUnknownFieldAndBadCase(t *testing.T) {
	srv := httptest.NewServer(NewServer().Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/screen", "application/json", bytes.NewBufferString(`{"base_mva":100,"unknown":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", resp.StatusCode)
	}
}
