package config

import (
	"errors"
	"io"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	ZoneFile    string `toml:"zone_file"`
	Port        int
	Rndc        string
	Path        string
	LogToSyslog bool     `toml:"syslog"`
	SSLEnabled  bool     `toml:"ssl_enabled"`
	SSLCert     string   `toml:"ssl_cert"`
	SSLKey      string   `toml:"ssl_key"`
	Clients     []Client `toml:"client"`
}

var fieldNames map[string]string = make(map[string]string)

func init() {
	// Build the map of representation names to internal names
	rt := reflect.TypeOf(Config{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)

		field := strings.ToLower(f.Name)
		if tag := f.Tag.Get("toml"); tag != "" {
			field = tag
		}

		fieldNames[field] = f.Name
	}
}

type Client struct {
	Username string
	Password string
}

func LoadFile(file string) (Config, error) {
	var config Config
	if _, err := toml.DecodeFile(file, &config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func LoadReader(r io.Reader) (Config, error) {
	var config Config
	if _, err := toml.DecodeReader(r, &config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (config *Config) Dump(w io.Writer) error {
	return toml.NewEncoder(w).Encode(config)
}

func (config *Config) Override(variable string, value interface{}) error {
	if field, ok := fieldNames[variable]; ok {
		variable = field
	} else {
		return errors.New("Variable \"" + variable + "\" is invalid.")
	}

	s := reflect.ValueOf(config).Elem()
	f := s.FieldByName(variable)
	if !f.IsValid() || !f.CanSet() {
		return errors.New("This variable does not exist/cannot be set")
	}
	f.Set(reflect.ValueOf(value))
	return nil
}
