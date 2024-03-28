# Cinema service
[![Go Report Card](https://goreportcard.com/badge/github.com/Falokut/cinema_orders_service)](https://goreportcard.com/report/github.com/Falokut/cinema_orders_service)
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/Falokut/cinema_orders_service)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/Falokut/cinema_orders_service)
[![Go](https://github.com/Falokut/cinema_orders_service/actions/workflows/go.yml/badge.svg?branch=master)](https://github.com/Falokut/cinema_orders_service/actions/workflows/go.yml) ![](https://changkun.de/urlstat?mode=github&repo=Falokut/cinema_orders_service)
[![License](https://img.shields.io/badge/license-MIT-green)](./LICENSE)
---

# Content

+ [Configuration](#configuration)
    + [Params info](#configuration-params-info)
        + [time.Duration](#timeduration-yaml-supported-values)
        + [Database config](#database-config)
        + [Jaeger config](#jaeger-config)
        + [Prometheus config](#prometheus-config)
        + [Secure connection config](#secure-connection-config)
+ [Metrics](#metrics)
+ [Docs](#docs)
+ [Author](#author)
+ [License](#license)
---------

# Configuration

1. [Configure cinema_orders_db](cinema_orders_db/README.md#Configuration)
2. Create .env on project root dir  
Example env:
```env
DB_CONNECTION_STRING=url
REDIS_PASSWORD=password
REDIS_AOF_ENABLED=no
```
3. Create a configuration file or change the config.yml file in docker\containers-configs.
If you are creating a new configuration file, specify the path to it in docker-compose volume section (your-path/config.yml:configs/)

## Configuration params info
if supported values is empty, then any type values are supported

| yml name | yml section | env name | param type| description | supported values |
|-|-|-|-|-|-|
| log_level   |      | LOG_LEVEL  |   string   |      logging level        | panic, fatal, error, warning, warn, info, debug, trace|
| host   |  listen    | HOST  |   string   |  ip address or host to listen   |  |
| port   |  listen    | PORT  |   string   |  port to listen   | The string should not contain delimiters, only the port number|
| server_mode   |  listen    | SERVER_MODE  |   string   | Server listen mode, Rest API, gRPC or both | GRPC, REST, BOTH|
| allowed_headers   |  listen    |  |   []string, array of strings   | list of all allowed custom headers. Need for REST API gateway, list of metadata headers, hat are passed through the gateway into the service | any strings list|
| healthcheck_port   |      | HEALTHCHECK_PORT  |   string   |     port for healthcheck| any valid port that is not occupied by other services. The string should not contain delimiters, only the port number|
|service_name|  prometheus    | PROMETHEUS_SERVICE_NAME | string |  service name, thats will show in prometheus  ||
|server_config|  prometheus    |   | nested yml configuration  [metrics server config](#prometheus-config) | |
|db_config|||nested yml configuration  [database config](#database-config) || configuration for database connection | |
|jaeger|||nested yml configuration  [jaeger config](#jaeger-config)|configuration for jaeger connection ||
| network   | reserve_cache     | RESERVE_CACHE_NETWORK  |   string   |     network type       | tcp or udp|
| addr   |   reserve_cache   | RESERVE_CACHE_ADDR  |   string   |   ip address(or host) with port of redis| all valid addresses formatted like host:port or ip-address:port |
|password| reserve_cache|RESERVE_CACHE_PASSWORD|string|password for connection to the redis||
|db| reserve_cache|RESERVE_CACHE_DB|string|the number of the database in the redis||
| network   | screening_reserve_cache     | SCREENING_RESERVE_CACHE_NETWORK  |   string   |     network type       | tcp or udp|
| addr   |   screening_reserve_cache   | SCREENING_RESERVE_CACHE_ADDR  |   string   |   ip address(or host) with port of redis| all valid addresses formatted like host:port or ip-address:port |
|password| screening_reserve_cache|SCREENING_RESERVE_CACHE_PASSWORD|string|password for connection to the redis||
|db| screening_reserve_cache|SCREENING_RESERVE_CACHE_DB|string|the number of the database in the redis||
| reservation_time   |      |  |  time.Duration with positive duration | the time that screening places reservation will be stored in the cache, seat reservation time|[supported values](#timeduration-yaml-supported-values)|
|db| screening_reserve_cache|SCREENING_RESERVE_CACHE_DB|string|the number of the database in the redis||
|db_name||DB_NAME|string|database name||
|db_connection_string||DB_CONNECTION_STRING|string|database connection string||
|payment_url|payment_service|PAYMENT_URL|string|url for payment stub page||
|payment_sleep_time|payment_service||time.Duration with positive duration|the time after which the order status changes to PAID|[supported values](#timeduration-yaml-supported-values)|
|refund_sleep_time|payment_service||time.Duration with positive duration|the time after which the order status changes TO REFUNDED|[supported values](#timeduration-yaml-supported-values)|
|addr|profiles_service|PROFILES_SERVICE_ADDR|string|address of the profiles service|all valid addresses formatted like host:port or ip-address:port|
|secure_config|profiles_service||nested yml configuration  [secure config](#secure-connection-config)|||
|addr|cinema_orders_service|CINEMA_SERVICE_ADDR|string|address of the profiles service|all valid addresses formatted like host:port or ip-address:port|
|secure_config|cinema_orders_service||nested yml configuration  [secure config](#secure-connection-config)|||


### time.Duration yaml supported values
A Duration value can be expressed in various formats, such as in seconds, minutes, hours, or even in nanoseconds. Here are some examples of valid Duration values:
- 5s represents a duration of 5 seconds.
- 1m30s represents a duration of 1 minute and 30 seconds.
- 2h represents a duration of 2 hours.
- 500ms represents a duration of 500 milliseconds.
- 100µs represents a duration of 100 microseconds.
- 10ns represents a duration of 10 nanoseconds.

### Jaeger config
|yml name| env name|param type| description | supported values |
|-|-|-|-|-|
|address|JAEGER_ADDRESS|string|ip address(or host) with port of jaeger service| all valid addresses formatted like host:port or ip-address:port |
|service_name|JAEGER_SERVICE_NAME|string|service name, thats will show in jaeger in traces||
|log_spans|JAEGER_LOG_SPANS|bool|whether to enable log scans in jaeger for this service or not||

### Prometheus config
|yml name| env name|param type| description | supported values |
|-|-|-|-|-|
|host|METRIC_HOST|string|ip address or host to listen for prometheus service||
|port|METRIC_PORT|string|port to listen for  of prometheus service| any valid port that is not occupied by other services. The string should not contain delimiters, only the port number|


### Secure connection config
|yml name| param type| description | supported values |
|-|-|-|-|
|dial_method|string|dial method|INSECURE,NIL_TLS_CONFIG,CLIENT_WITH_SYSTEM_CERT_POOL,SERVER|
|server_name|string|server name overriding, used when dial_method=CLIENT_WITH_SYSTEM_CERT_POOL||
|cert_name|string|certificate file name, used when dial_method=SERVER||
|key_name|string|key file name, used when dial_method=SERVER||

# Metrics
The service uses Prometheus and Jaeger and supports distribution tracing

# Docs
[Swagger docs](swagger/docs/cinema_orders_service_v1.swagger.json)

# Author

- [@Falokut](https://github.com/Falokut) - Primary author of the project

# License

This project is licensed under the terms of the [MIT License](https://opensource.org/licenses/MIT).

---
