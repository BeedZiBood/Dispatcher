package config

import (
	"flag"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env               string `yaml:"env"`
	PostgresConfig    `yaml:"postgres"`
	HTTPServer        `yaml:"http_server"`
	GRPCClient        `yaml:"grpc_config"`
	KafkaProducer     `yaml:"kafka_producer"`
	CycleBufferConfig `yaml:"cycle_buffer"`
}

type HTTPServer struct {
	Address     string        `yaml:"address"`
	Timeout     time.Duration `yaml:"timeout"`
	IdleTimeout time.Duration `yaml:"idle_timeout"`
}

type GRPCClient struct {
	Address string        `yaml:"address"`
	Timeout time.Duration `yaml:"timeout"`
}

type KafkaProducer struct {
	Broker string `yaml:"broker"`
	Topic  string `yaml:"topic"`
}

type PostgresConfig struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	DBName   string `yaml:"db_name"`
	SSLMode  string `yaml:"ssl_mode"`
}

type CycleBufferConfig struct {
	MaxSize int `yaml:"max_size"`
}

func MustLoad() *Config {
	configPath := fetchConfigPath()

	if configPath == "" {
		panic("config path is empty")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		panic("config path doesn't exist: " + configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		panic("failed to read config: " + err.Error())
	}

	return &cfg
}

func fetchConfigPath() string {
	var result string

	flag.StringVar(&result, "config", "", "path to the config")
	flag.Parse()

	if result == "" {
		result = os.Getenv("CONFIG_PATH")
	}

	return result
}
