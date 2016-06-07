package bind

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sync"
)

type Zone string
type Master string
type ZoneMap map[Zone]Master
type ZoneSet map[Zone]bool

type ZoneAdderFunc func(zd *ZoneManager, master Master, zone Zone) error
type ZoneRemoverFunc func(zd *ZoneManager, master Master, zone Zone) error

type masters map[Master]ZoneSet

type ZoneManager struct {
	masters  masters
	zones    ZoneMap
	lock     sync.RWMutex
	adder    ZoneAdderFunc
	remover  ZoneRemoverFunc
	dataPath string
	rndcPath string
}

func (zd *ZoneManager) AddZone(master Master, zone Zone) error {
	zd.lock.Lock()
	defer zd.lock.Unlock()

	if currentMaster, present := zd.zones[zone]; present {
		return fmt.Errorf(`Zone "%s" is already assigned to "%s"`, zone, currentMaster)
	}

	result := zd.adder(zd, master, zone)
	if result != nil {
		return result
	}

	if zd.masters[master] == nil {
		zd.masters[master] = ZoneSet{}
	}
	zd.masters[master][zone] = true
	zd.zones[zone] = master

	return nil
}

func RndcZoneAdder(zd *ZoneManager, master Master, zone Zone) error {
	data := fmt.Sprintf(`{type slave; file "slave/%s.db"; masters { %s; };};`, zone, master)
	cmd := exec.Command(zd.rndcPath, "addzone", string(zone), data)
	output, err := cmd.CombinedOutput()
	log.Printf(`[exec] %s %s %s '%s': %s`, zd.rndcPath, "addzone", string(zone), data, string(output))
	return err
}

func NullZoneAdder(zd *ZoneManager, master Master, zone Zone) error {
	return nil
}

func LogZoneAdder(zd *ZoneManager, master Master, zone Zone) error {
	data := fmt.Sprintf(`{type slave; file "slave/%s.db"; masters { %s; };};`, zone, master)
	log.Printf("[Skipped execution] %s %s %s '%s'", zd.rndcPath, "addzone", string(zone), data)
	return nil
}

func LoadingZoneAdder(zd *ZoneManager, master Master, zone Zone) error {
	log.Printf(`Loaded zone %s with master %s`, zone, master)
	return nil
}

func (zd *ZoneManager) RemoveZone(master Master, zone Zone) error {
	zd.lock.Lock()
	defer zd.lock.Unlock()

	if _, present := zd.zones[zone]; !present {
		return errors.New("not found in zone map")
	}

	if _, present := zd.masters[master]; !present {
		return errors.New("master not found")
	}

	if _, present := zd.masters[master][zone]; !present {
		return errors.New("zone not in this master")
	}

	delete(zd.zones, zone)
	delete(zd.masters[master], zone)
	if len(zd.masters[master]) == 0 {
		delete(zd.masters, master)
	}
	return zd.remover(zd, master, zone)
}

func RndcZoneRemover(zd *ZoneManager, master Master, zone Zone) error {
	cmd := exec.Command(zd.rndcPath, "delzone", string(zone))
	if err := cmd.Run(); err != nil {
		return err
	}

	// It's ok if the zone does not exist
	path := fmt.Sprintf(`%s/slave/%s.db`, zd.dataPath, zone)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}

	// But remove it if it does
	return os.Remove(path)
}

func NullZoneRemover(zd *ZoneManager, master Master, zone Zone) error {
	return nil
}

func LogZoneRemover(zd *ZoneManager, master Master, zone Zone) error {
	log.Printf("[Skipped execution] %s %s %s", zd.rndcPath, "delzone", string(zone))
	return nil
}

func (zd *ZoneManager) GetMasters() []Master {
	zd.lock.RLock()
	defer zd.lock.RUnlock()

	masters := make([]Master, 0, len(zd.masters))
	for master, _ := range zd.masters {
		masters = append(masters, master)
	}
	return masters
}

func (zd *ZoneManager) GetZones(master Master) ZoneSet {
	zd.lock.RLock()
	defer zd.lock.RUnlock()

	zones := make(ZoneSet, len(zd.masters[master]))
	for zone, _ := range zd.masters[master] {
		zones[zone] = true
	}
	return zones
}

func (zd *ZoneManager) GetZoneMap() ZoneMap {
	zd.lock.RLock()
	defer zd.lock.RUnlock()

	zones := make(ZoneMap, len(zd.zones))
	for zone, master := range zd.zones {
		zones[zone] = master
	}
	return zones
}

func NewZoneManager() *ZoneManager {
	zones := ZoneManager{
		masters:  make(masters),
		zones:    make(ZoneMap),
		adder:    NullZoneRemover,
		remover:  NullZoneRemover,
		dataPath: "./",
		rndcPath: "./rndc",
	}

	return &zones
}

func (zd *ZoneManager) LoadZones(reader io.Reader) {
	oldAdder := zd.adder
	zd.adder = LoadingZoneAdder

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		found, master, zone := parseLine(scanner.Text())
		if found {
			zd.AddZone(master, zone)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}

	zd.adder = oldAdder
}

func (zd *ZoneManager) ZoneAdder(adder ZoneAdderFunc) {
	zd.adder = adder
}
func (zd *ZoneManager) ZoneRemover(remover ZoneRemoverFunc) {
	zd.remover = remover
}
func (zd *ZoneManager) Path(path string) {
	zd.dataPath = path
}
func (zd *ZoneManager) RndcPath(path string) {
	zd.rndcPath = path
}

var bindLine = regexp.MustCompile(`zone "?([^\s"{]+)"?\s*{type\s+slave;\s*file\s+"[^"]+";\s*masters\s*{\s*([^;]+)\s*;\s*}\s*;\s*}\s*;`)
var ignoredLine = regexp.MustCompile(`^\s*(#.*)?$`)

func parseLine(line string) (found bool, master Master, zone Zone) {
	if ignoredLine.MatchString(line) {
		return false, "", ""
	}

	matches := bindLine.FindStringSubmatch(line)
	if matches == nil {
		panic("Invalid config line \"" + line + "\"")
	}
	return true, Master(matches[2]), Zone(matches[1])
}
