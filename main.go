package main

import (
    "bufio"
    "fmt"
    "log"
    "net"
    "os"
    "runtime"
    "runtime/debug"
    "strconv"
    "strings"
    "sync"
    "time"
)

type User struct {
    Username         string
    Password         string
    StartDate        time.Time
    EndDate          time.Time
    ConnectionLimit  int
    MaxData          int64  // in bytes
    MaxBandwidth     int64  // in bytes per second
    CurrentDataUsage int64
    CurrentConns     int
}

type SystemConfig struct {
    MaxConnections    int
    MaxBandwidth      int64
    ConnectionTimeout int
    GCPercent         int
}

var (
    users        map[string]*User
    usersMutex   sync.RWMutex // Mutex to protect access to users map
    systemConfig SystemConfig
    userFile     = "users.conf"   // Path to users.conf
    systemFile   = "system.conf"  // Path to system.conf"
)

// Load system configuration from system.conf
func loadSystemConfig(filePath string) error {
    file, err := os.Open(filePath)
    if err != nil {
        return err
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := scanner.Text()
        if len(line) == 0 || strings.HasPrefix(line, "#") {
            continue
        }

        parts := strings.Split(line, "=")
        if len(parts) != 2 {
            continue
        }

        key := strings.TrimSpace(parts[0])
        value := strings.TrimSpace(parts[1])

        switch key {
        case "max_connections":
            maxConns, err := strconv.Atoi(value)
            if err != nil {
                return fmt.Errorf("invalid max_connections value: %v", err)
            }
            systemConfig.MaxConnections = maxConns

        case "max_bandwidth":
            maxBW, err := strconv.ParseInt(value, 10, 64)
            if err != nil {
                return fmt.Errorf("invalid max_bandwidth value: %v", err)
            }
            systemConfig.MaxBandwidth = maxBW

        case "connection_timeout":
            timeout, err := strconv.Atoi(value)
            if err != nil {
                return fmt.Errorf("invalid connection_timeout value: %v", err)
            }
            systemConfig.ConnectionTimeout = timeout

        case "gc_percent":
            gcPercent, err := strconv.Atoi(value)
            if err != nil {
                return fmt.Errorf("invalid gc_percent value: %v", err)
            }
            systemConfig.GCPercent = gcPercent

        default:
            log.Printf("Unknown configuration key: %s", key)
        }
    }

    if err := scanner.Err(); err != nil {
        return err
    }

    log.Println("System configuration loaded successfully.")
    return nil
}

// Load users from users.conf
func loadUsers(filePath string) error {
    file, err := os.Open(filePath)
    if err != nil {
        return err
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    newUsers := make(map[string]*User) // Temporary user map

    for scanner.Scan() {
        line := scanner.Text()
        parts := strings.Split(line, ",")
        if len(parts) != 7 {
            continue
        }

        startDate, _ := time.Parse("2006-01-02", parts[2])
        endDate, _ := time.Parse("2006-01-02", parts[3])
        connectionLimit, _ := strconv.Atoi(parts[4])
        maxData, _ := strconv.ParseInt(parts[5], 10, 64)
        maxBandwidth, _ := strconv.ParseInt(parts[6], 10, 64)

        newUsers[parts[0]] = &User{
            Username:        parts[0],
            Password:        parts[1],
            StartDate:       startDate,
            EndDate:         endDate,
            ConnectionLimit: connectionLimit,
            MaxData:         maxData,
            MaxBandwidth:    maxBandwidth,
        }
    }

    if err := scanner.Err(); err != nil {
        return err
    }

    // Lock the users map and update it with the new data
    usersMutex.Lock()
    users = newUsers
    usersMutex.Unlock()

    log.Println("User list reloaded successfully.")
    return nil
}

// Function to periodically reload the user list every 10 seconds
func autoReloadUsers() {
    for {
        time.Sleep(10 * time.Second)
        err := loadUsers(userFile)
        if err != nil {
            log.Printf("Error reloading users: %v", err)
        }
    }
}

func handleConnection(conn net.Conn) {
    defer conn.Close()
    fmt.Fprintf(conn, "Kết nối thành công!\n")
}

func acceptConnections() {
    listener, err := net.Listen("tcp", ":8080")
    if err != nil {
        log.Fatalf("Không thể khởi tạo listener: %v", err)
    }
    defer listener.Close()

    for {
        conn, err := listener.Accept()
        if err != nil {
            log.Printf("Lỗi kết nối: %v", err)
            continue
        }

        go handleConnection(conn)
    }
}

func main() {
    // Tải cấu hình hệ thống từ file system.conf
    err := loadSystemConfig(systemFile)
    if err != nil {
        log.Fatalf("Không thể tải cấu hình hệ thống: %v", err)
    }

    // Tối ưu hệ thống theo cấu hình đã tải
    runtime.GOMAXPROCS(4)  // Sử dụng tối đa 4 CPU cores
    debug.SetGCPercent(systemConfig.GCPercent)  // Tăng giá trị GC từ cấu hình

    log.Printf("Max connections: %d", systemConfig.MaxConnections)
    log.Printf("Max bandwidth: %d", systemConfig.MaxBandwidth)
    log.Printf("Connection timeout: %d", systemConfig.ConnectionTimeout)

    // Load users từ file users.conf
    err = loadUsers(userFile)
    if err != nil {
        log.Fatalf("Không thể tải danh sách người dùng: %v", err)
    }

    // Goroutine tự động reload danh sách user sau mỗi 10 giây
    go autoReloadUsers()

    // Khởi động server TCP
    acceptConnections()
}
