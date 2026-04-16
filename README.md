# gzgspd

Guangzhou College of Technology and Business portal login daemon

I want you to be able to tell what it is at a glance.

## Download & Run

[GitHub Releases](https://github.com/SummonHIM/gzgspd/releases/latest)

### Arguments

- `--config`: Specify configuration file path.
- `--version`: Display current version of gzgspd.
- `--test`: Test configuration and exit.

### Run as service

- Linux/OpenWrt: [File](files/services)
- Windows: [NSSM](https://nssm.cc/download)

## Configures

The configuration file uses JSON format, as shown below:

```Json
{
  "log_level": 0,           // Log level (https://go.dev/src/log/slog/level.go)
  "log_path": "daemon.log", // Log path
  "instance": [             // Instances
    {
      "username": "13412345678",           // User name
      "password": "123456",                // Password
      "interface": "",                     // Network interface for sending HTTP data (Empty: Automatically detect)
      "keep_alive": 5,                     // User agent for sending HTTP data (Empty: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
      "keep_alive_link": "http://3.3.3.3", // keep-alive link (Empty: "http://3.3.3.3")
      "retry_max": 3,                      // Max retries. If exceeded, wait 10 minutes.
      "retry_time": 5                      // Retry interval
    }
  ]
}
```

and its struct is defined as follows:

```Go
type Config struct {
	LogLevel int              `json:"log_level"` // Log level (https://go.dev/src/log/slog/level.go)
	LogPath  string           `json:"log_path"`  // Log path
	Instance []ConfigInstance `json:"instance"`  // Instances
}

type ConfigInstance struct {
	Username   string `json:"username"`        // User name
	Password   string `json:"password"`        // Password
	Interface  string `json:"interface"`       // Network interface for sending HTTP data (Empty: Automatically detect)
	UserAgent  string `json:"user_agent"`      // User agent for sending HTTP data (Empty: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	KeepAlive  int    `json:"keep_alive"`      // Interval for sending keep-alive
	KAliveLink string `json:"keep_alive_link"` // keep-alive link (Empty: "http://3.3.3.3")
	RetryMax   int    `json:"retry_max"`       // Max retries. If exceeded, wait 10 minutes.
	RetryTime  int    `json:"retry_time"`      // Retry interval
}
```
