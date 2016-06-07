package config_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kilburn/bdns/config"
)

const testFile = "../test/config.toml"

func testConfig() config.Config {
	return config.Config{
		ZoneFile:    "3bf305731dd26307.nzf",
		Port:        54515,
		Rndc:        "test/rndc.sh",
		Path:        "test/",
		LogToSyslog: false,
		SSLEnabled:  true,
		SSLCert:     "test/ssl/cert.bundle.crt",
		SSLKey:      "test/ssl/cert.key",
		Clients: []config.Client{
			config.Client{Username: "client1", Password: "password1"},
			config.Client{Username: "client2", Password: "password2"},
		},
	}
}

func TestLoadFile(t *testing.T) {
	expected := testConfig()

	conf, ok := config.LoadFile(testFile)
	if ok != nil {
		t.Fatalf("Unable to load config file \"%s\": %v", testFile, ok)
	}

	compare(t, conf, expected)
}

func TestLoadReader(t *testing.T) {
	expected := config.Config{
		SSLEnabled: true,
	}

	input := strings.NewReader("ssl_enabled = true")
	conf, ok := config.LoadReader(input)
	if ok != nil {
		t.Fatalf("Unable to load config (%v).", ok)
	}

	compare(t, conf, expected)
}

func TestOverride(t *testing.T) {
	expected := testConfig()
	expected.Port = 2

	conf, ok := config.LoadFile(testFile)
	if ok != nil {
		t.Fatalf("Unable to load config file \"%s\": %v", testFile, ok)
	}

	ok = conf.Override("port", 2)
	if ok != nil {
		t.Fatalf("Unable to load config file \"%s\": %v", testFile, ok)
	}

	compare(t, conf, expected)
}

func TestDump(t *testing.T) {
	expected := testConfig()
	buffer := new(bytes.Buffer)
	ok := expected.Dump(buffer)
	if ok != nil {
		t.Fatalf("Unable to dump configuration (%v).", ok)
	}

	conf, ok := config.LoadReader(buffer)
	if ok != nil {
		t.Fatalf("Unable to load configuration (%v)", ok)
	}

	compare(t, conf, expected)
}

func compare(t *testing.T, result, expected config.Config) {
	resultStr, _ := json.Marshal(result)
	expectedStr, _ := json.Marshal(expected)
	if !bytes.Equal(resultStr, expectedStr) {
		t.Errorf("Unexpected output (%v != %v)", result, expected)
	}
}
