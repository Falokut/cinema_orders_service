package config

import (
	"crypto/tls"
	"crypto/x509"
	"sync"
	"time"

	"github.com/Falokut/cinema_orders_service/pkg/jaeger"
	"github.com/Falokut/cinema_orders_service/pkg/metrics"
	logging "github.com/Falokut/online_cinema_ticket_office.loggerwrapper"
	"github.com/ilyakaznacheev/cleanenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	LogLevel        string `yaml:"log_level" env:"LOG_LEVEL"`
	HealthcheckPort string `yaml:"healthcheck_port" env:"HEALTHCHECK_PORT"`
	Listen          struct {
		Host           string   `yaml:"host" env:"HOST"`
		Port           string   `yaml:"port" env:"PORT"`
		Mode           string   `yaml:"server_mode" env:"SERVER_MODE"` // support GRPC, REST, BOTH
		AllowedHeaders []string `yaml:"allowed_headers"`
	} `yaml:"listen"`

	PrometheusConfig struct {
		Name         string                      `yaml:"service_name" ENV:"PROMETHEUS_SERVICE_NAME"`
		ServerConfig metrics.MetricsServerConfig `yaml:"server_config"`
	} `yaml:"prometheus"`

	DbConnectionString string        `yaml:"db_connection_string" env:"DB_CONNECTION_STRING"`
	DbName       string        `yaml:"db_name" env:"DB_NAME"`
	JaegerConfig       jaeger.Config `yaml:"jaeger"`
	ReserveCache       struct {
		Network  string `yaml:"network" env:"RESERVE_CACHE_NETWORK"`
		Addr     string `yaml:"addr" env:"RESERVE_CACHE_ADDR"`
		DB       int    `yaml:"db" env:"RESERVE_CACHE_DB"`
		Password string `yaml:"password" env:"RESERVE_CACHE_PASSWORD"`
	} `yaml:"reserve_cache"`

	SeatReservationTime time.Duration `yaml:"seat_reservation_time"`

	CinemaServiceConfig struct {
		Addr         string                 `yaml:"addr" env:"CINEMA_SERVICE_ADDR"`
		SecureConfig ConnectionSecureConfig `yaml:"secure_config"`
	} `yaml:"cinema_service"`

	ProfilesServiceConfig struct {
		Addr         string                 `yaml:"addr" env:"PROFILES_SERVICE_ADDR"`
		SecureConfig ConnectionSecureConfig `yaml:"secure_config"`
	} `yaml:"profiles_service"`

	PaymentServiceConfig struct {
		Paymenturl       string        `yaml:"payment_url" env:"PAYMENT_URL"`
		PaymentSleepTime time.Duration `yaml:"payment_sleep_time"`
		RefundSleepTime  time.Duration `yaml:"refund_sleep_time"`
	} `yaml:"payment_service"`

	OrdersEventsConfig struct {
		Brokers []string `yaml:"brokers"`
	} `yaml:"orders_events"`
}

var instance *Config
var once sync.Once

const configsPath = "configs/"

func GetConfig() *Config {
	once.Do(func() {
		logger := logging.GetLogger()
		instance = &Config{}

		if err := cleanenv.ReadConfig(configsPath+"config.yml", instance); err != nil {
			help, _ := cleanenv.GetDescription(instance, nil)
			logger.Fatal(help, " ", err)
		}
	})

	return instance
}

type DialMethod = string

const (
	Insecure                 DialMethod = "INSECURE"
	NilTlsConfig             DialMethod = "NIL_TLS_CONFIG"
	ClientWithSystemCertPool DialMethod = "CLIENT_WITH_SYSTEM_CERT_POOL"
	Server                   DialMethod = "SERVER"
)

type ConnectionSecureConfig struct {
	Method DialMethod `yaml:"dial_method"`
	// Only for client connection with system pool
	ServerName string `yaml:"server_name"`
	CertName   string `yaml:"cert_name"`
	KeyName    string `yaml:"key_name"`
}

func (c ConnectionSecureConfig) GetGrpcTransportCredentials() (grpc.DialOption, error) {
	if c.Method == Insecure {
		return grpc.WithTransportCredentials(insecure.NewCredentials()), nil
	}

	if c.Method == NilTlsConfig {
		return grpc.WithTransportCredentials(credentials.NewTLS(nil)), nil
	}

	if c.Method == ClientWithSystemCertPool {
		certPool, err := x509.SystemCertPool()
		if err != nil {
			return grpc.EmptyDialOption{}, err
		}
		return grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(certPool, c.ServerName)), nil
	}

	cert, err := tls.LoadX509KeyPair(c.CertName, c.KeyName)
	if err != nil {
		return grpc.EmptyDialOption{}, err
	}
	return grpc.WithTransportCredentials(credentials.NewServerTLSFromCert(&cert)), nil
}
