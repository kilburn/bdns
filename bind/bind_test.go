package bind_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

import "github.com/kilburn/bdns/bind"

func TestNewZoneManager(t *testing.T) {
	zd := bind.NewZoneManager()
	masters := zd.GetMasters()
	if len(masters) != 0 {
		t.Errorf("Expected an empty list of masters, got %v", masters)
	}
}

func TestLoadZones(t *testing.T) {
	const input = `
# Empty and uncommented lines should be ignored
zone "test.com" {type slave; file "slave/test.com.db"; masters { 192.168.2.10; };};
zone domain.tld {type slave; file "slave/domain.es.db"; masters { 10.10.29.19; };};
zone something.net {type slave; file "slave/something.net.db"; masters { 127.0.3.1; };};`
	zd := bind.NewZoneManager()
	zd.LoadZones(strings.NewReader(input))

	output := bind.NewZoneManager()
	output.AddZone("192.168.2.10", "test.com")
	output.AddZone("127.0.3.1", "something.net")
	output.AddZone("10.10.29.19", "domain.tld")
	compare(t, zd, output)
}

func TestLoadZones_Invalid(t *testing.T) {
	const input = `zone basis.es`

	defer func() {
		r := recover()
		if r == nil {
			t.Error("Parsing finished without error when it should have panicked.")
		} else if r != `Invalid config line "`+input+`"` {
			t.Errorf("Got unexpected error: %v", r)
		}
	}()
	zd := bind.NewZoneManager()
	zd.LoadZones(strings.NewReader(input))
}

func TestAddZone(t *testing.T) {
	zone := bind.Zone("domain.tld")
	master := bind.Master("178.33.115.135")

	zd := bind.NewZoneManager()
	zd.AddZone(master, zone)
	result := zd.GetZones(master)
	if len(result) != 1 {
		t.Errorf(`Expected 1 zone, got %d (%v)`, len(result), result)
	}
	if !result[zone] {
		t.Errorf(`Expected "%s" zone, got "%s"`, zone, result)
	}
}

func TestAddExistingZone(t *testing.T) {
	zone := bind.Zone("domain.tld")
	master1 := bind.Master("178.33.115.135")
	master2 := bind.Master("178.33.115.135")

	zd := bind.NewZoneManager()
	if err := zd.AddZone(master1, zone); err != nil {
		t.Errorf(`Got error "%v" when adding "%v" to "%v"`, err, zone, master1)
	}
	if err := zd.AddZone(master1, zone); err == nil {
		t.Errorf(`Expected error when adding "%v" to "%v", but didn't get it`, err, zone, master2)
	}
	result := zd.GetZones(master1)
	if !result[zone] {
		t.Errorf(`Expected "%s" zone, got "%s"`, zone, result)
	}
}

func TestRemoveZone(t *testing.T) {
	zd := bind.NewZoneManager()
	zd.AddZone("178.33.115.135", "basis.es")
	ok := zd.RemoveZone("178.33.115.135", "basis.es")
	if ok != nil {
		t.Errorf(`Error reming zone (%v)`, ok)
	}
	compare(t, zd, bind.NewZoneManager())
}

func TestGetMasters(t *testing.T) {
	zd := bind.NewZoneManager()
	masters := zd.GetMasters()
	if len(masters) != 0 {
		t.Errorf("Got masters from an empty zone data: %b", masters)
	}

	zd.AddZone("178.33.115.135", "basis.es")
	masters = zd.GetMasters()
	if len(masters) != 1 {
		t.Errorf("Expected a single master, got something else: %#v (%d)", masters, len(masters))
	}
}

func TestGetZones(t *testing.T) {
	const (
		m1 = bind.Master("192.168.2.1")
		m2 = bind.Master("192.168.2.2")
		m3 = bind.Master("192.168.2.3")
		z1 = bind.Zone("domain.org")
		z2 = bind.Zone("domain.com")
		z3 = bind.Zone("domain.es")
	)

	zd := bind.NewZoneManager()
	zd.AddZone(m1, z1)
	zd.AddZone(m2, z2)
	zd.AddZone(m2, z3)

	zones := zd.GetZones(m1)
	compareZoneSet(t, zones, bind.ZoneSet{z1: true})

	zones = zd.GetZones(m2)
	compareZoneSet(t, zones, bind.ZoneSet{z2: true, z3: true})

	zones = zd.GetZones(m3)
	compareZoneSet(t, zones, bind.ZoneSet{})
}

func compareZoneSet(t *testing.T, result, expected bind.ZoneSet) {
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Got a different set than expected (%v != %v)", result, expected)
	}
}

func compareZoneMap(t *testing.T, result, expected bind.ZoneMap) {
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Got a different map than expected (%v != %v)", result, expected)
	}
}

func compare(t *testing.T, result, expected *bind.ZoneManager) {
	for _, master := range result.GetMasters() {
		compareZoneSet(t, result.GetZones(master), expected.GetZones(master))
	}
	compareZoneMap(t, result.GetZoneMap(), expected.GetZoneMap())
	resultStr, _ := json.Marshal(result)
	expectedStr, _ := json.Marshal(expected)
	if !bytes.Equal(resultStr, expectedStr) {
		t.Errorf("Unexpected output (%v != %v)", result, expected)
	}
}
