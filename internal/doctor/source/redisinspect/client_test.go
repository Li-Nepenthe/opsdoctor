// 本文件：用一个只会回复固定消息的 Redis 协议小服务器检查适配器。
//
// 不需要 Docker，也不碰业务缓存。测试服务器只认识握手、INFO 和 SLOWLOG，
// 多出别的命令就报错，因此能检查客户端是否偷偷执行了内容查询。
//
// 主要内容：
//   - TestReadContract：验证 INFO 解析与慢日志参数删减。
//   - TestConfiguration：拒绝匿名、业务账号、错误数据库和无效分区。
//   - fakeRedis、readCommand：实现本测试用到的最小 RESP 协议。
//
// 阅读建议：RESP 用长度标记字符串，因此测试参数里即使有空格也不会被拆错。

package redisinspect

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
)

// TestReadContract 保留耗时证据，同时确认缓存键和值没有进入输出。
func TestReadContract(t *testing.T) {
	address := fakeRedis(t)
	client, err := New("redis://diag_ro:demo-only@" + address + "/0")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	info, err := client.Info(t.Context(), "stats")
	if err != nil || info["keyspace_hits"] != "42" || info["address"] != "localhost:6379" {
		t.Fatalf("INFO=%v err=%v", info, err)
	}
	slow, err := client.Info(t.Context(), "slowlog")
	if err != nil {
		t.Fatal(err)
	}
	text := slow["slowlog_summary"]
	for _, secret := range []string{"private-key", "private-value"} {
		if strings.Contains(text, secret) {
			t.Fatal("慢日志泄露了参数")
		}
	}
	if !strings.Contains(text, `"command":"SET"`) || !strings.Contains(text, `"arguments":2`) || !strings.Contains(text, `"duration_us":250`) {
		t.Fatal(text)
	}
	// 已取消的请求应保留取消原因，调用方才能把它和账号错误区分开。
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Info(ctx, "stats"); err != context.Canceled {
		t.Fatalf("取消结果=%v", err)
	}
}

// TestConfiguration 在联网前拒绝危险配置，非法 INFO 分区也不能被拼成别的命令。
func TestConfiguration(t *testing.T) {
	for _, address := range []string{
		"redis://localhost:6379",                 // 匿名连接绕过账号隔离。
		"redis://default:demo@localhost:6379",    // 业务账号权限过大。
		"redis://diag_ro@localhost:6379",         // 只填账号仍未提供认证信息。
		"redis://diag_ro:demo@localhost:6379/15", // 医生不应切换到测试库。
		"not-a-url",                              // 错误格式不能变成默认连接。
	} {
		if _, err := New(address); err == nil {
			t.Fatal("接受了错误配置")
		}
	}
	client, err := New("redis://diag_ro:demo@127.0.0.1:1/0")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Info(t.Context(), "stats\r\nGET secret"); err == nil {
		t.Fatal("接受了非法分区")
	}
	if _, err := parseInfo("broken-line"); err == nil {
		t.Fatal("接受了错误 INFO")
	}
}

// fakeRedis 只开本机临时端口；清理连接后等待服务退出，避免测试留下 goroutine。
func fakeRedis(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		for {
			args, err := readCommand(reader)
			if err != nil {
				return
			}
			response := "-ERR unexpected command\r\n"
			switch strings.ToUpper(args[0]) {
			case "HELLO":
				response = "-ERR unknown command HELLO\r\n" // 覆盖旧协议回退握手。
			case "AUTH":
				response = "+OK\r\n"
			case "INFO":
				body := "# Stats\r\nkeyspace_hits:42\r\naddress:localhost:6379\r\n"
				response = fmt.Sprintf("$%d\r\n%s\r\n", len(body), body)
			case "SLOWLOG":
				response = "*1\r\n*6\r\n:1\r\n:1700000000\r\n:250\r\n*3\r\n+SET\r\n+private-key\r\n+private-value\r\n+127.0.0.1\r\n+client\r\n"
			}
			if _, err := io.WriteString(connection, response); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })
	return listener.Addr().String()
}

// readCommand 只解析客户端发送的字符串数组，不实现测试不需要的 Redis 功能。
func readCommand(reader *bufio.Reader) ([]string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(header, "*") {
		return nil, fmt.Errorf("expected array")
	}
	count, err := strconv.Atoi(strings.TrimSpace(header[1:]))
	if err != nil || count < 1 || count > 16 {
		return nil, fmt.Errorf("invalid argument count")
	}
	result := make([]string, 0, count)
	for range count {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(header, "$") {
			return nil, fmt.Errorf("expected bulk string")
		}
		length, err := strconv.Atoi(strings.TrimSpace(header[1:]))
		if err != nil || length < 0 || length > 4096 {
			return nil, fmt.Errorf("invalid length")
		}
		// 后两个字节是协议换行，不能算进参数本身。
		body := make([]byte, length+2)
		if _, err := io.ReadFull(reader, body); err != nil {
			return nil, err
		}
		result = append(result, string(body[:length]))
	}
	return result, nil
}
