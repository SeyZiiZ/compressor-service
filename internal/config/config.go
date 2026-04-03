package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	RabbitMQ RabbitMQConfig
	Storage  StorageConfig
	Worker   WorkerConfig
}

type ServerConfig struct {
	Port         int    `mapstructure:"port"`
	Host         string `mapstructure:"host"`
	MaxUploadMB  int    `mapstructure:"max_upload_mb"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

func (d DatabaseConfig) DSN() string {
	return "host=" + d.Host +
		" port=" + strings.TrimSpace(viper.GetString("database.port")) +
		" user=" + d.User +
		" password=" + d.Password +
		" dbname=" + d.DBName +
		" sslmode=" + d.SSLMode
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type RabbitMQConfig struct {
	URL string `mapstructure:"url"`
}

type StorageConfig struct {
	BasePath string `mapstructure:"base_path"`
}

type WorkerConfig struct {
	VideoWorkers int `mapstructure:"video_workers"`
	ImageWorkers int `mapstructure:"image_workers"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/compressor/")

	// Defaults
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.max_upload_mb", 500)

	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "compressor")
	viper.SetDefault("database.password", "compressor")
	viper.SetDefault("database.dbname", "compressor")
	viper.SetDefault("database.sslmode", "disable")

	viper.SetDefault("redis.addr", "localhost:6379")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)

	viper.SetDefault("rabbitmq.url", "amqp://guest:guest@localhost:5672/")

	viper.SetDefault("storage.base_path", "./data")

	viper.SetDefault("worker.video_workers", 2)
	viper.SetDefault("worker.image_workers", 4)

	// Env vars: COMPRESSOR_SERVER_PORT, COMPRESSOR_DATABASE_HOST, etc.
	viper.SetEnvPrefix("COMPRESSOR")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Try reading config file (optional)
	_ = viper.ReadInConfig()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
