package config

import (
	"flag"
	"os"
)

type Flags struct {
	ConfigFile        string
	ActionShowVersion bool
	ActionTestConfig  bool
}

func ParseFlags(flags *Flags) {
	defaultConfigFile := "config.json"
	if v, ok := os.LookupEnv("GSGZPD_CONFIG_FILE"); ok && v != "" {
		defaultConfigFile = v
	}

	flag.StringVar(&flags.ConfigFile, "config", defaultConfigFile, "Specify the configuration file path.")
	flag.BoolVar(&flags.ActionShowVersion, "version", false, "Display current version of gzgspd.")
	flag.BoolVar(&flags.ActionTestConfig, "test", false, "Test configuration and exit.")
	flag.Parse()
}
