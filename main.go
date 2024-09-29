package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Cấu trúc thông tin người dùng
type User struct {
	Username         string
	Password         string
	StartDate        time.Time
	EndDate          time.Time
	ConnectionLimit  int
	MaxData          int64  // Giới hạn dữ liệu (tính bằng byte)
	MaxBandwidth     int64  // Băng thông tối đa (tính bằng byte/giây)
	CurrentDataUsage int64  // Lượng dữ liệu đã sử dụng (tính bằng byte)
	CurrentConns     int    // Số lượng kết nối hiện tại
}

type SystemConfig struct {
	MaxConnections    int   // Tổng số kết nối tối đa
	MaxBandwidth      int64 // Băng thông tối đa (byte/giây)
	ConnectionTimeout int   // Thời gian timeout kết nối (giây)
	GCPercent         int   // Tỉ lệ thu gom rác
}

var (
	users        map[string]*User
	usersMutex   sync.RWMutex // Bảo vệ truy cập đến map `users`
	systemConfig SystemConfig
	serverRunning = false
	serverListener net.Listener
	wg sync.WaitGroup
	userFile     = "users.conf"   // Đường dẫn đến file `users.conf`
	systemFile   = "system.conf"  // Đường dẫn đến file `system.conf`
	ipv6ProxyList []string        // Lưu danh sách proxy IPv6
	ipv4ProxyList []string        // Lưu danh sách proxy IPv4
)

// Load cấu hình hệ thống từ file
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

// Load user từ file
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

// Xác thực người dùng dựa trên username và password
func authenticateUser(username, password string) (*User, bool) {
	usersMutex.RLock()
	defer usersMutex.RUnlock()

	user, exists := users[username]
	if !exists {
		return nil, false // Không tồn tại user
	}

	if user.Password != password {
		return nil, false // Sai password
	}

	// Kiểm tra xem người dùng có vượt quá giới hạn số lượng kết nối không
	if user.CurrentConns >= user.ConnectionLimit {
		return nil, false
	}

	return user, true
}

// Kiểm tra và cập nhật băng thông
func trackBandwidth(user *User, dataSize int64) bool {
	user.CurrentDataUsage += dataSize
	if user.CurrentDataUsage > user.MaxData {
		return false // Quá giới hạn dữ liệu
	}

	// Cần thêm logic kiểm tra băng thông nếu cần
	return true
}

// Xử lý kết nối SOCKS4
func handleSocks4(conn net.Conn, user *User) {
	defer conn.Close()

	// Đọc yêu cầu SOCKS4
	buf := make([]byte, 8)
	if _, err := conn.Read(buf); err != nil {
		log.Printf("SOCKS4 Read Error: %v", err)
		return
	}

	if buf[0] != 0x04 {
		log.Println("Not a SOCKS4 connection")
		return
	}

	// Kiểm tra yêu cầu kết nối (CONNECT command = 0x01)
	if buf[1] != 0x01 {
		conn.Write([]byte{0x00, 0x5B}) // Chỉ hỗ trợ lệnh CONNECT
		return
	}

	// Địa chỉ IP và cổng của địa chỉ đích
	port := binary.BigEndian.Uint16(buf[2:4])
	destIP := net.IPv4(buf[4], buf[5], buf[6], buf[7])

	// Đọc username và bỏ qua
	for {
		b := make([]byte, 1)
		if _, err := conn.Read(b); err != nil || b[0] == 0x00 {
			break
		}
	}

	// Kết nối tới địa chỉ đích
	destAddr := fmt.Sprintf("%s:%d", destIP.String(), port)
	targetConn, err := net.DialTimeout("tcp", destAddr, time.Duration(systemConfig.ConnectionTimeout)*time.Second)
	if err != nil {
		conn.Write([]byte{0x00, 0x5B}) // Không thể kết nối
		return
	}
	defer targetConn.Close()

	conn.Write([]byte{0x00, 0x5A}) // Xác nhận kết nối thành công

	// Truyền dữ liệu giữa client và đích
	transferData(conn, targetConn, user)
}

// Xử lý kết nối SOCKS5 với xác thực username/password
func handleSocks5(conn net.Conn, user *User) {
	defer conn.Close()

	// Bước 1: Handshake
	buf := make([]byte, 2)
	if _, err := conn.Read(buf); err != nil {
		log.Printf("SOCKS5 Read Error: %v", err)
		return
	}

	if buf[0] != 0x05 {
		log.Println("Not a SOCKS5 connection")
		return
	}

	// Bỏ qua các phương thức xác thực, vì chúng ta đã yêu cầu xác thực username/password
	authMethods := make([]byte, int(buf[1]))
	if _, err := conn.Read(authMethods); err != nil {
		log.Printf("SOCKS5 Read Auth Methods Error: %v", err)
		return
	}

	// Trả về rằng yêu cầu xác thực username/password (mã 0x02)
	conn.Write([]byte{0x05, 0x02})

	// Bước 2: Xác thực username/password
	buf = make([]byte, 2)
	if _, err := conn.Read(buf); err != nil {
		log.Printf("SOCKS5 Authentication Error: %v", err)
		return
	}

	usernameLen := int(buf[1])
	username := make([]byte, usernameLen)
	if _, err := conn.Read(username); err != nil {
		log.Printf("SOCKS5 Read Username Error: %v", err)
		return
	}

	buf = make([]byte, 1)
	if _, err := conn.Read(buf); err != nil {
		log.Printf("SOCKS5 Password Length Error: %v", err)
		return
	}

	passwordLen := int(buf[0])
	password := make([]byte, passwordLen)
	if _, err := conn.Read(password); err != nil {
		log.Printf("SOCKS5 Read Password Error: %v", err)
		return
	}

	// Xác thực người dùng
	if _, authenticated := authenticateUser(string(username), string(password)); !authenticated {
		conn.Write([]byte{0x01, 0x01}) // Trả về mã lỗi xác thực
		return
	}

	conn.Write([]byte{0x01, 0x00}) // Xác thực thành công

	// Bước 3: Xử lý yêu cầu kết nối
	buf = make([]byte, 4)
	if _, err := conn.Read(buf); err != nil {
		log.Printf("SOCKS5 Request Error: %v", err)
		return
	}

	if buf[1] != 0x01 {
		conn.Write([]byte{0x05, 0x07}) // Chỉ hỗ trợ lệnh CONNECT
		return
	}

	// Địa chỉ đích (IPv4, IPv6, domain name)
	var destAddr string
	switch buf[3] {
	case 0x01: // IPv4
		ip := make([]byte, 4)
		if _, err := conn.Read(ip); err != nil {
			log.Printf("SOCKS5 Read IPv4 Error: %v", err)
			return
		}
		portBuf := make([]byte, 2)
		if _, err := conn.Read(portBuf); err != nil {
			log.Printf("SOCKS5 Read Port Error: %v", err)
			return
		}
		port := binary.BigEndian.Uint16(portBuf)
		destAddr = fmt.Sprintf("%s:%d", net.IP(ip).String(), port)

	case 0x04: // IPv6
		ip := make([]byte, 16)
		if _, err := conn.Read(ip); err != nil {
			log.Printf("SOCKS5 Read IPv6 Error: %v", err)
			return
		}
		portBuf := make([]byte, 2)
		if _, err := conn.Read(portBuf); err != nil {
			log.Printf("SOCKS5 Read Port Error: %v", err)
			return
		}
		port := binary.BigEndian.Uint16(portBuf)
		destAddr = fmt.Sprintf("[%s]:%d", net.IP(ip).String(), port)
	}

	// Kết nối tới địa chỉ đích
	targetConn, err := net.DialTimeout("tcp", destAddr, time.Duration(systemConfig.ConnectionTimeout)*time.Second)
	if err != nil {
		conn.Write([]byte{0x05, 0x04}) // Lỗi kết nối
		return
	}
	defer targetConn.Close()

	// Trả về thành công kết nối
	conn.Write([]byte{0x05, 0x00, 0x00, buf[3]})

	// Truyền dữ liệu giữa client và đích
	transferData(conn, targetConn, user)
}

// Truyền dữ liệu giữa client và server đích với giới hạn băng thông
func transferData(src, dst net.Conn, user *User) {
	// Giới hạn băng thông và theo dõi dữ liệu
	go io.Copy(dst, io.LimitReader(src, user.MaxBandwidth))
	io.Copy(src, io.LimitReader(dst, user.MaxBandwidth))
}

func startServer(ip string, port int) {
	addr := fmt.Sprintf("%s:%d", ip, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Cannot start server on %s: %v", addr, err)
	}
	serverListener = listener
	serverRunning = true
	log.Printf("Server started on %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			if !serverRunning {
				break
			}
			continue
		}

		// Đọc phiên bản SOCKS để phân biệt SOCKS4 và SOCKS5
		version := make([]byte, 1)
		if _, err := conn.Read(version); err != nil {
			conn.Close()
			continue
		}

		if version[0] == 0x04 {
			go handleSocks4(conn, nil) // SOCKS4
		} else if version[0] == 0x05 {
			go handleSocks5(conn, nil) // SOCKS5 với xác thực username/password
		} else {
			conn.Close() // Không hỗ trợ phiên bản khác
		}
	}
}

func stopServer() {
	serverRunning = false
	if serverListener != nil {
		serverListener.Close()
		log.Println("Server stopped.")
	}
}

func showMenu() {
	for {
		fmt.Println("Menu:")
		fmt.Println("1. Trạng thái server")
		fmt.Println("2. Tạo Proxy/Socks4/Socks5 cho IPv4")
		fmt.Println("3. Tạo Proxy/Socks4/Socks5 cho IPv6")
		fmt.Println("4. Dừng server")
		fmt.Println("5. Danh sách Proxy/Socks4/Socks5 cho IPv6")
		fmt.Print("Chọn tùy chọn: ")

		var choice int
		fmt.Scan(&choice)

		switch choice {
		case 1:
			// Trạng thái server
			if serverRunning {
				fmt.Println("Server đang chạy.")
			} else {
				fmt.Println("Server đã dừng.")
			}
		case 2:
			// Tạo Proxy IPv4
			startServer("0.0.0.0", 1080)
		case 3:
			// Tạo Proxy IPv6
			startServer("::", 1080)
		case 4:
			// Dừng server
			stopServer()
		case 5:
			// Hiển thị danh sách proxy IPv6
			fmt.Println("Danh sách proxy IPv6:", ipv6ProxyList)
		default:
			fmt.Println("Tùy chọn không hợp lệ. Vui lòng chọn lại.")
		}
	}
}

func main() {
	// Load system config và users
	err := loadSystemConfig(systemFile)
	if err != nil {
		log.Fatalf("Unable to load system configuration: %v", err)
	}
	err = loadUsers(userFile)
	if err != nil {
		log.Fatalf("Unable to load user list: %v", err)
	}

	// Bắt đầu menu điều khiển server
	showMenu()
}
