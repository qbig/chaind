package config

import (
	"github.com/spf13/viper"
	"path"
	"os"
	"fmt"
	"github.com/mitchellh/go-homedir"
	"errors"
)

const DefaultHome = "~/.chaind"
const DefaultConfigFile = "chaind.toml"

const (
	FlagHome     = "home"
	FlagDBUrl    = "db_url"
	FlagCertPath = "cert_path"
	FlagUseTLS   = "use_tls"
	FlagBTCURL   = "btc_path"
	FlagETHURL   = "eth_path"
	FlagRPCPort  = "rpc_port"
)

type Config struct {
	Home             string            `mapstructure:"home"`
	DBUrl            string            `mapstructure:"db_url"`
	CertPath         string            `mapstructure:"cert_path"`
	UseTLS           bool              `mapstructure:"use_tls"`
	BTCUrl           string            `mapstructure:"btc_url"`
	ETHUrl           string            `mapstructure:"eth_url"`
	RPCPort          int               `mapstructure:"rpc_port"`
	LogLevel         string            `mapstructure:"log_level"`
	LogAuditorConfig *LogAuditorConfig `mapstructure:"log_auditor"`
	RedisConfig      *RedisConfig      `mapstructure:"redis"`
}

type LogAuditorConfig struct {
	LogFile string `mapstructure:"log_file"`
}

type RedisConfig struct {
	URL      string `mapstructure:"url"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

func init() {
	home := mustExpand(DefaultHome)
	viper.SetDefault(FlagHome, home)
	viper.SetDefault(FlagDBUrl, fmt.Sprintf("file:%s/chaind.db", home))
	viper.SetDefault(FlagCertPath, "")
	viper.SetDefault(FlagUseTLS, false)
	viper.SetDefault(FlagBTCURL, "btc")
	viper.SetDefault(FlagETHURL, "eth")
	viper.SetDefault(FlagRPCPort, 8080)
}

func ReadConfig(allowDefaults bool) (Config, error) {
	var cfg Config
	cfgFile := path.Join(viper.GetString(FlagHome), DefaultConfigFile)
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		if allowDefaults {
			viper.Unmarshal(&cfg)
			return cfg, nil
		} else {
			return cfg, errors.New("config file not found")
		}
	}

	viper.SetConfigFile(cfgFile)
	if err := viper.ReadInConfig(); err != nil {
		return cfg, err
	}
	if err := viper.Unmarshal(&cfg); err != nil {
		return cfg, err
	}
	viper.Set(FlagHome, mustExpand(viper.GetString(FlagHome)))
	viper.Set(FlagDBUrl, mustExpand(viper.GetString(FlagDBUrl)))
	viper.Set(FlagCertPath, mustExpand(viper.GetString(FlagCertPath)))

	return cfg, nil
}

func mustExpand(path string) string {
	expanded, err := homedir.Expand(path)
	if err != nil {
		fmt.Println("Failed to find home directory on this system. Exiting.")
		os.Exit(1)
	}

	return expanded
}
