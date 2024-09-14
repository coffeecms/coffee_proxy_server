# Go Proxy Server with User Management - https://blog.lowlevelforest.com/

This is a high-performance proxy server written in Go that supports user management with configurable options. The server is designed to handle a large number of simultaneous connections efficiently using Go’s concurrency features.

## Features

- **User Management**: Load user credentials and their specific settings from a configuration file.
- **Dynamic User Reload**: Automatically reload the user list every 10 seconds.
- **System Configuration**: Load system-wide configuration settings from a separate configuration file.
- **Concurrency**: Utilize Go's goroutines for handling multiple connections simultaneously.
- **Garbage Collection Tuning**: Adjust garbage collection settings based on system configuration.

## Prerequisites

- Go 1.18 or later
- Access to a terminal or command line

## Installation

1. **Clone the repository**:
   ```bash
   git clone https://github.com/coffeecms/coffee_proxy_server.git
   cd coffee_proxy_server
   ```

2. **Build the project**:
   ```bash
   go build -o proxy-server main.go
   ```

3. **Create configuration files**:
   - `system.conf`: Contains system-wide configuration settings.
   - `users.conf`: Contains user credentials and their specific settings.

   Example `system.conf`:
   ```ini
   max_connections=1000000
   max_bandwidth=1000000000
   connection_timeout=60
   gc_percent=100
   ```

   Example `users.conf`:
   ```csv
   username1,password1,2023-01-01,2024-01-01,10,1000000,50000
   username2,password2,2023-06-01,2024-06-01,5,500000,25000
   ```

## Usage

1. **Run the server**:
   ```bash
   ./proxy-server
   ```

   The server will listen on port 8080 by default.

2. **Modify user and system configurations** as needed and restart the server for changes to take effect.

## Configuration Files

### `system.conf`

- `max_connections`: Maximum number of simultaneous connections.
- `max_bandwidth`: Maximum allowable bandwidth (in bytes per second).
- `connection_timeout`: Timeout for connections (in seconds).
- `gc_percent`: Garbage collection percent (higher value means less frequent GC).

### `users.conf`

- `username`: Username for authentication.
- `password`: Password for authentication.
- `start_date`: User account start date (YYYY-MM-DD).
- `end_date`: User account expiration date (YYYY-MM-DD).
- `connection_limit`: Maximum number of connections allowed for the user.
- `max_data`: Maximum data usage allowed for the user (in bytes).
- `max_bandwidth`: Maximum bandwidth usage allowed for the user (in bytes per second).

## Contribution

Contributions are welcome! Please feel free to submit pull requests or open issues.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contact

For any questions or issues, please contact [lowlevelforest@gmail.com](mailto:lowlevelforest@gmail.com).
