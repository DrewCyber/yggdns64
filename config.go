package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

type InvalidAddress int64

const (
	IgnoreInvalidAddress  InvalidAddress = 0
	ProcessInvalidAddress InvalidAddress = 1
	DiscardInvalidAddress InvalidAddress = 2
)

type ZoneConfig struct {
	Domains          []string `yaml:"domains"`
	Prefix           net.IP   `yaml:"prefix,omitempty"`
	ReturnPublicIPv4 bool     `yaml:"return-public-ipv4"`
}

type Config struct {
	Listen     string                `yaml:"listen"`
	Zones      map[string]ZoneConfig `yaml:"zones"`
	Forwarders map[string]string     `yaml:"forwarders"`
	Default    string                `yaml:"default"`
	IA         InvalidAddress        `yaml:"invalid-address"`
	Static     map[string]string     `yaml:"static"`
	Cache      struct {
		ExpTime   time.Duration `yaml:"expiration"`
		PurgeTime time.Duration `yaml:"purge"`
	} `yaml:"cache"`
	LogLevel string `yaml:"log-level"`
}

func (a InvalidAddress) String() string {
	switch a {
	case IgnoreInvalidAddress:
		return "Ignore"
	case ProcessInvalidAddress:
		return "Process"
	case DiscardInvalidAddress:
		return "Discard"
	}
	return "Ignore"
}

func (ia *InvalidAddress) UnmarshalYAML(unmarshal func(interface{}) error) (err error) {
	var IA string

	err = unmarshal(&IA)
	if err != nil {
		return
	}

	switch strings.ToLower(IA) {
	case "ignore":
		*ia = IgnoreInvalidAddress
	case "process":
		*ia = ProcessInvalidAddress
	case "discard":
		*ia = DiscardInvalidAddress
	default:
		return fmt.Errorf("invalid-address must be one of 'ignore/process/discard'")
	}

	return nil
}

func InitConfig() (Config, error) {
	fileName := flag.String("file", "config.yml", "config filename")
	flag.Parse()

	Configs, err := parseFile(*fileName)
	if err != nil {
		return Config{}, err
	}
	return *Configs, nil
}

func parseFile(filePath string) (*Config, error) {
	cfg := new(Config)
	body, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	cfg.Cache.ExpTime = 0
	cfg.Cache.PurgeTime = 0
	cfg.LogLevel = "info"
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// func (c *Config) validateForwarders() {
// }
