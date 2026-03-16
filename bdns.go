package main

import (
	"flag"
	"fmt"
	"log"
	"log/syslog"
	"net"
	"net/http"
	"os"
	"os/exec"     // <--- AFEGIR AQUEST
    	"path/filepath" // <--- AFEGIR AQUEST per gestionar rutes netament
	"strconv"
	"strings"

	"github.com/kilburn/bdns/bind"
	"github.com/kilburn/bdns/config"
)

func isValidUser(username string, password string) bool {
	for _, client := range conf.Clients {
		if client.Username == username && client.Password == password {
			return true
		}
	}
	return false
}

func authHandler(fn func(w http.ResponseWriter, r *http.Request, host bind.Master)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			_ = recover()
		}()

		// Extraiem la IP real (per si venim de Nginx)
		hostAddr := r.Header.Get("X-Real-IP")
		if hostAddr == "" {
			// Si no hi ha header, agafem la IP de la connexió directa
			var err error
			hostAddr, _, err = net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				http.Error(w, "Unable to establish origin.", http.StatusForbidden)
				return
			}
		}

		// Autenticació
		username, password, ok := r.BasicAuth()
		if !ok || !isValidUser(username, password) {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"bdns\"")
			http.Error(w, "Invalid credentials.", http.StatusUnauthorized)
			log.Panicf("Unauthorized request from %v\n", hostAddr)
		}

		log.Printf("[%s] %s %s\n", hostAddr, r.Method, r.URL.Path)
		w.Header().Set("Content-type", "text/plain")
		fn(w, r, bind.Master(hostAddr))
	}
}

func mainHandler(w http.ResponseWriter, r *http.Request, host bind.Master) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	for _, master := range zoneManager.GetMasters() {
		for zone, _ := range zoneManager.GetZones(master) {
			fmt.Fprintf(w, "%s\t%s\n", zone, master)
		}
	}
}

func fetchZone(w http.ResponseWriter, r *http.Request) bind.Zone {
	parts := strings.Split(r.URL.Path, "/")
	if parts == nil || len(parts) != 3 || parts[2] == "" {
		http.Error(w, "Invalid zone.", http.StatusBadRequest)
		log.Panicf(`Invalid zone "%v"`, parts[2:])
	}
	return bind.Zone(parts[2])
}

func addHandler(w http.ResponseWriter, r *http.Request, host bind.Master) {
	zone := fetchZone(w, r)
	if ok := zoneManager.AddZone(host, zone); ok != nil {
		msg := fmt.Sprintf(`Error adding zone %v (%s)`, zone, ok.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		log.Panic(msg)
	}
	fmt.Fprint(w, "OK")
}

func removeHandler(w http.ResponseWriter, r *http.Request, host bind.Master) {
	zone := fetchZone(w, r)
	if ok := zoneManager.RemoveZone(host, zone); ok != nil {
		msg := fmt.Sprintf(`Error removing zone %v (%s)`, zone, ok.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		log.Panic(msg)
	}
	fmt.Fprint(w, "OK")
}

func listHandler(w http.ResponseWriter, r *http.Request, host bind.Master) {
	for zone, _ := range zoneManager.GetZones(host) {
		fmt.Fprintf(w, "%s\n", zone)
	}
}

var zoneManager *bind.ZoneManager

func setupLogging() {
	// Log to syslog if applicable
	if conf.LogToSyslog {
		w, err := syslog.New(syslog.LOG_NOTICE|syslog.LOG_DAEMON, "bdns")
		if err != nil {
			log.Panicf("Unable to initialize syslogging (%s)", err.Error())
		}
		log.SetOutput(w)
		log.SetFlags(0)
	}
}

var conf config.Config

func loadConfiguration() {
	file := flag.Lookup("configfile").Value.String()
	var ok error
	if conf, ok = config.LoadFile(file); ok != nil {
		fmt.Printf("Unable to load configuration: %v\n", ok)
		os.Exit(1)
	}

	flag.Visit(func(a *flag.Flag) {
		g := a.Value.(flag.Getter)
		conf.Override(a.Name, g.Get())
	})

	getter := (flag.Lookup("dumpconfig").Value).(flag.Getter)
	if (getter.Get()).(bool) {
		conf.Dump(os.Stderr)
		os.Exit(0)
	}
}

func loadZoneManager() {
	log.Print("Loading zones from BIND directly (Nut2 Modern Mode)")

	zoneManager = bind.NewZoneManager()
	zoneManager.ZoneAdder(bind.RndcZoneAdder)
	zoneManager.ZoneRemover(bind.RndcZoneRemover)
	
	// Validació de rutes per evitar punters nuls
	if conf.Path == "" {
		conf.Path = "/var/cache/bind"
	}
	if conf.Rndc == "" {
		conf.Rndc = "/usr/sbin/rndc"
	}
	
	zoneManager.Path(conf.Path)
	zoneManager.RndcPath(conf.Rndc)

	// Inicialitzem el manager amb un lector buit
	zoneManager.LoadZones(strings.NewReader(""))

	slavePath := filepath.Join(conf.Path, "slave")
	files, err := os.ReadDir(slavePath)
	if err != nil {
		log.Printf("Error llegint directori slave: %v", err)
		return
	}

	count := 0
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".db") {
			zoneName := strings.TrimSuffix(file.Name(), ".db")

			// Executem rndc showzone
			out, err := exec.Command(conf.Rndc, "showzone", zoneName).Output()
			if err != nil {
				continue
			}

			// NETEJA DE LA LÍNIA: 
			// 1. Convertim el resultat a text
			// 2. Eliminem salts de línia i tabuladors per fer-ho una sola línia plana
			line := strings.ReplaceAll(string(out), "\n", " ")
			line = strings.ReplaceAll(line, "\r", " ")
			line = strings.ReplaceAll(line, "\t", " ")
			line = strings.TrimSpace(line)

			// 3. Ens assegurem que la línia conté el necessari per a la regex
			if strings.Contains(line, "type slave") && strings.Contains(line, "masters") {
				// PROTECCIÓ CONTRA PANICS:
				// Si la regex de bind.go falla, el defer recover evitarà que el programa mori
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[Warning] Error parsejant zona %s: %v", zoneName, r)
						}
					}()
					zoneManager.LoadZones(strings.NewReader(line))
					count++
				}()
			}
		}
	}
	log.Printf("Done loading %d zones from BIND via RNDC", count)
}

func serve() {
	http.HandleFunc("/list/", authHandler(listHandler))
	http.HandleFunc("/add/", authHandler(addHandler))
	http.HandleFunc("/remove/", authHandler(removeHandler))
	http.HandleFunc("/", authHandler(mainHandler))

	if conf.SSLEnabled {
		server := &http.Server{
			Addr: ":" + strconv.Itoa(conf.Port),
		}
		go func() {
			log.Fatal(server.ListenAndServeTLS(conf.SSLCert, conf.SSLKey))
		}()
	}

	server := &http.Server{
		Addr: ":" + strconv.Itoa(conf.Port-1),
	}
	log.Fatal(server.ListenAndServe())
}

func init() {
	flag.Bool("dumpconfig", false, "Dumps the effective configuration settings and terminates")
	flag.String("configfile", "/etc/bdns/bdns.conf", "Set the configuration file to use")
	flag.String("zonefile", "3bf305731dd26307.nzf", "Set the bind's zone file to read")
	flag.Int("port", 54515, "Port where to listen")
	flag.String("rndc", "/usr/sbin/rndc", "Path to the rndc executable")
	flag.String("path", "/var/cache/bind", "Path to bind's data directory")
	flag.Bool("syslog", false, "Send logs to syslog")
	flag.Bool("ssl_enabled", false, "Enables https")
	flag.Bool("ssl_cert", false, "Path to the certificate (bundle)")
	flag.Bool("ssl_key", false, "Path to the certificate key")
}

func main() {
	flag.Parse()
	loadConfiguration()
	setupLogging()
	loadZoneManager()
	serve()
}
