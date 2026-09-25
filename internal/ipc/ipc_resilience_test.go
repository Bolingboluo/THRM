package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSlowRequestDoesNotBlockOthers 是 #1 的回归测试。
//
// 修复前服务端按连接串行处理请求，一个慢请求（设备就绪握手最长 6s、
// 桥接故障探测最长 10s）会把同一条连接上后续所有请求一起拖过对端超时阈值，
// 对端连续超时后拆掉整条连接重连，表现为"核心服务不可用"误报。
func TestSlowRequestDoesNotBlockOthers(t *testing.T) {
	release := make(chan struct{})
	server := NewServer(func(req Request) Response {
		if req.Type == ReqConnect {
			<-release // 模拟长时间设备连接
		}
		return Response{Success: true}
	}, testLogger{})
	if err := server.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer server.Stop()

	client := NewClient(testLogger{})
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer client.Close()

	var wg sync.WaitGroup
	wg.Go(func() {
		if _, err := client.SendRequestWithTimeout(ReqConnect, nil, 30*time.Second); err != nil {
			t.Errorf("慢请求失败: %v", err)
		}
	})

	// 等慢请求确实进入处理阶段。
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	resp, err := client.SendRequestWithTimeout(ReqPing, nil, 6*time.Second)
	elapsed := time.Since(start)

	close(release)
	wg.Wait()

	if err != nil {
		t.Fatalf("慢请求处理期间 Ping 失败: %v", err)
	}
	if resp == nil || !resp.Success {
		t.Fatalf("Ping 响应异常: %+v", resp)
	}
	if elapsed > time.Second {
		t.Fatalf("队头阻塞未修复：Ping 被慢请求阻塞了 %v", elapsed)
	}
	t.Logf("慢请求处理期间 Ping 耗时 %v（修复前会被阻塞到慢请求结束）", elapsed.Round(time.Millisecond))
}

// TestConcurrentSlowRequestsAllComplete 验证多个慢请求可以并发推进，
// 总耗时接近单个慢请求而不是逐个相加。
func TestConcurrentSlowRequestsAllComplete(t *testing.T) {
	const slow = 400 * time.Millisecond
	const count = 8

	server := NewServer(func(req Request) Response {
		time.Sleep(slow)
		return Response{Success: true}
	}, testLogger{})
	if err := server.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer server.Stop()

	client := NewClient(testLogger{})
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer client.Close()

	start := time.Now()
	var failures atomic.Int64
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() {
			if _, err := client.SendRequestWithTimeout(ReqPing, nil, 10*time.Second); err != nil {
				failures.Add(1)
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)

	if failures.Load() > 0 {
		t.Fatalf("%d/%d 个并发请求失败", failures.Load(), count)
	}
	// 串行时为 count*slow；允许调度开销，取一半作为门槛。
	if elapsed > time.Duration(count)*slow/2 {
		t.Fatalf("%d 个并发慢请求耗时 %v，看起来仍在串行处理", count, elapsed)
	}
	t.Logf("%d 个 %v 的慢请求并发完成，总耗时 %v（串行需 %v）",
		count, slow, elapsed.Round(time.Millisecond), time.Duration(count)*slow)
}

// TestHandlerPanicReturnsErrorResponse 验证 handler panic 被兜底。
// 请求现在在独立协程里执行，未捕获的 panic 会直接终止整个核心进程。
func TestHandlerPanicReturnsErrorResponse(t *testing.T) {
	server := NewServer(func(req Request) Response {
		if req.Type == ReqGetDebugInfo {
			panic("boom")
		}
		return Response{Success: true}
	}, testLogger{})
	if err := server.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer server.Stop()

	client := NewClient(testLogger{})
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer client.Close()

	resp, err := client.SendRequestWithTimeout(ReqGetDebugInfo, nil, 5*time.Second)
	if err != nil {
		t.Fatalf("panic 请求应返回错误响应而不是超时: %v", err)
	}
	if resp.Success {
		t.Fatal("panic 请求不应返回成功")
	}
	if resp.ErrorCode != "internal_panic" {
		t.Fatalf("ErrorCode = %q, want internal_panic", resp.ErrorCode)
	}

	// 连接必须仍然可用。
	if follow, err := client.SendRequestWithTimeout(ReqPing, nil, 5*time.Second); err != nil || !follow.Success {
		t.Fatalf("panic 之后连接不可用: resp=%+v err=%v", follow, err)
	}
}

// TestStalledClientIsEvictedByWriteTimeout 是 #3 的回归测试。
//
// 模拟一个连上就再也不读的对端（进程假死但未退出）。修复前 clientWriter 的
// conn.Write 没有 deadline，会永久阻塞：客户端条目永远留在 clients 表里，
// HasClients() 恒为 true，核心因此永远无法回到空闲低频模式。
func TestStalledClientIsEvictedByWriteTimeout(t *testing.T) {
	old := clientWriteTimeout
	clientWriteTimeout = 300 * time.Millisecond
	defer func() { clientWriteTimeout = old }()

	server := NewServer(func(req Request) Response {
		return Response{Success: true}
	}, testLogger{})
	if err := server.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer server.Stop()

	// 裸连接：只连上，从不读取。
	var conn net.Conn
	var err error
	for _, pipeName := range ipcPipeCandidates() {
		conn, err = dialIPC(ipcEndpointFromName(pipeName), 2*time.Second)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	// 等服务端登记该客户端。
	deadline := time.Now().Add(2 * time.Second)
	for !server.HasClients() {
		if time.Now().After(deadline) {
			t.Fatal("服务端未登记客户端")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 持续广播大事件，把管道缓冲与写队列填满，逼出写超时。
	payload := strings.Repeat("x", 64*1024)
	evicted := false
	for range 400 {
		server.BroadcastEvent(EventConfigUpdate, payload)
		if !server.HasClients() {
			evicted = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !evicted {
		t.Fatal("假死客户端未被写超时摘除，HasClients() 仍为 true —— 核心将永远停在非空闲模式")
	}
	t.Log("假死客户端已被写超时摘除，HasClients() 恢复为 false")
}

// TestClientWriteDeadlineFailsFast 验证客户端侧写入也带超时。
// 修复前，核心假死不再读取管道时，conn.Write 会在持有 c.mutex 的状态下永久阻塞，
// 后续所有请求跟着永久卡住——而请求超时只覆盖"等响应"，对此完全无效。
func TestClientWriteDeadlineFailsFast(t *testing.T) {
	// 只监听并接受连接，从不读取。
	listener, _, err := listenIPC()
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()

	client := NewClient(testLogger{})
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer client.Close()

	select {
	case conn := <-accepted:
		defer conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("服务端未接受连接")
	}

	// 大 payload 反复写入，直到管道缓冲填满触发写超时。
	payload := strings.Repeat("y", 256*1024)
	start := time.Now()
	var lastErr error
	for range 200 {
		_, lastErr = client.SendRequestWithTimeout(ReqUpdateConfig, payload, 500*time.Millisecond)
		if lastErr != nil && !errors.Is(lastErr, ErrRequestTimeout) {
			t.Fatalf("未预期的错误类型: %v", lastErr)
		}
		if lastErr != nil && !client.IsConnected() {
			// 写超时已把连接标记为失效，符合预期。
			t.Logf("写入在 %v 内超时并标记连接失效（修复前会永久阻塞）", time.Since(start).Round(time.Millisecond))
			return
		}
		if time.Since(start) > 20*time.Second {
			break
		}
	}

	// 未能填满缓冲时至少要确认请求本身以超时结束，而不是无限挂起。
	if lastErr == nil {
		t.Fatal("对端从不读取，请求却全部成功返回")
	}
	t.Logf("请求以超时结束（%v），未出现无限阻塞", lastErr)
}

// TestResponsesSurviveEventFlood 验证事件洪泛下响应仍然可靠送达。
// 响应与事件走不同队列，且响应不参与"队列满即丢"。
func TestResponsesSurviveEventFlood(t *testing.T) {
	server := NewServer(func(req Request) Response {
		data, _ := json.Marshal("pong")
		return Response{Success: true, Data: data}
	}, testLogger{})
	if err := server.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer server.Stop()

	client := NewClient(testLogger{})
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer client.Close()
	client.SetEventHandler(func(Event) {})

	stop := make(chan struct{})
	var flood sync.WaitGroup
	flood.Go(func() {
		payload := strings.Repeat("z", 16*1024)
		for {
			select {
			case <-stop:
				return
			default:
			}
			server.BroadcastEvent(EventConfigUpdate, payload)
		}
	})

	failures := 0
	const requests = 60
	for i := range requests {
		resp, err := client.SendRequestWithTimeout(ReqPing, nil, 5*time.Second)
		if err != nil {
			failures++
			t.Logf("请求 %d 失败: %v", i, err)
			continue
		}
		var got string
		if err := json.Unmarshal(resp.Data, &got); err != nil || got != "pong" {
			failures++
			t.Logf("请求 %d 响应损坏: err=%v data=%q", i, err, string(resp.Data))
		}
	}
	close(stop)
	flood.Wait()

	if failures > 0 {
		t.Fatalf("事件洪泛下 %d/%d 个请求失败", failures, requests)
	}
}

// TestBrokenFrameDoesNotDesyncStream 验证畸形请求不会打乱后续分帧。
func TestBrokenFrameDoesNotDesyncStream(t *testing.T) {
	server := NewServer(func(req Request) Response {
		return Response{Success: true}
	}, testLogger{})
	if err := server.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer server.Stop()

	var conn net.Conn
	var err error
	for _, pipeName := range ipcPipeCandidates() {
		conn, err = dialIPC(ipcEndpointFromName(pipeName), 2*time.Second)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("{not json}\n")); err != nil {
		t.Fatalf("write error: %v", err)
	}

	req, _ := json.Marshal(Request{RequestID: "probe-1", Type: ReqPing})
	if _, err := conn.Write(append(req, '\n')); err != nil {
		t.Fatalf("write error: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline error: %v", err)
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		t.Fatalf("畸形帧之后未能读到响应: %v", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("响应解析失败: %v (%q)", err, string(line))
	}
	if resp.RequestID != "probe-1" || !resp.Success {
		t.Fatalf("响应不匹配: %+v", resp)
	}
}

// TestPendingRequestFailsFastWhenConnectionDrops 验证连接断开时在途请求立即失败。
//
// 修复前 readLoop 退出时不清理 pending，等待者只能白等完整的超时（ReinstallPawnIO
// 这类长请求要等好几分钟）才发现连接早已不在。
func TestPendingRequestFailsFastWhenConnectionDrops(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := NewServer(func(req Request) Response {
		if req.Type == ReqConnect {
			close(entered)
			<-release
		}
		return Response{Success: true}
	}, testLogger{})
	if err := server.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer server.Stop()
	defer close(release)

	client := NewClient(testLogger{})
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer client.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := client.SendRequestWithTimeout(ReqConnect, nil, 30*time.Second)
		errCh <- err
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("请求未进入服务端处理")
	}

	start := time.Now()
	server.Stop() // 断开所有客户端连接，模拟核心退出/崩溃
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrConnectionClosed) {
			t.Fatalf("连接断开后的错误 = %v, want ErrConnectionClosed", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("在途请求在连接断开 %v 后才失败", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("在途请求没有随连接断开立即失败，仍在等待超时")
	}
	if client.IsConnected() {
		t.Fatal("连接断开后客户端仍报告已连接")
	}
}

// TestCloseGenerationSparesReplacementConnection 验证按代号关闭不会误伤新连接。
//
// 修复前 GUI 在请求失败后直接 Close()，关掉的是"当前"连接——若看护协程在这段时间里
// 已经重连成功，被拆掉的就是那条健康的新连接。
func TestCloseGenerationSparesReplacementConnection(t *testing.T) {
	server := NewServer(func(req Request) Response {
		return Response{Success: true}
	}, testLogger{})
	if err := server.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer server.Stop()

	client := NewClient(testLogger{})
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer client.Close()

	_, staleGen, err := client.SendRequestTracked(ReqPing, nil, 5*time.Second)
	if err != nil || staleGen == 0 {
		t.Fatalf("首个请求失败: gen=%d err=%v", staleGen, err)
	}

	client.Close()
	if err := client.Connect(); err != nil {
		t.Fatalf("重连失败: %v", err)
	}

	if client.CloseGeneration(staleGen) {
		t.Fatal("旧连接代号关闭了替换后的新连接")
	}
	if !client.IsConnected() {
		t.Fatal("旧连接代号导致新连接被标记为断开")
	}

	_, gen, err := client.SendRequestTracked(ReqPing, nil, 5*time.Second)
	if err != nil {
		t.Fatalf("新连接上的请求失败: %v", err)
	}
	if gen == staleGen {
		t.Fatalf("重连后连接代号未变化: %d", gen)
	}
	if !client.CloseGeneration(gen) {
		t.Fatal("当前连接代号未能关闭当前连接")
	}
	if client.IsConnected() {
		t.Fatal("CloseGeneration 之后仍报告已连接")
	}
}

// TestOversizedRequestFrameDropsConnection 验证请求帧有大小上限。
//
// 修复前服务端用 ReadBytes 读到换行为止，本机任意进程只发字节不发换行，
// 就能让以管理员身份运行的核心无限制地累积内存。
func TestOversizedRequestFrameDropsConnection(t *testing.T) {
	server := NewServer(func(req Request) Response {
		return Response{Success: true}
	}, testLogger{})
	if err := server.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer server.Stop()

	// 上限以内的大请求（远大于真实的完整配置）必须照常处理。
	client := NewClient(testLogger{})
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	resp, err := client.SendRequestWithTimeout(ReqUpdateConfig, strings.Repeat("c", 1<<20), 10*time.Second)
	client.Close()
	if err != nil || resp == nil || !resp.Success {
		t.Fatalf("上限以内的大请求失败: resp=%+v err=%v", resp, err)
	}

	var conn net.Conn
	for _, pipeName := range ipcPipeCandidates() {
		conn, err = dialIPC(ipcEndpointFromName(pipeName), 2*time.Second)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	// 超过上限且始终不带换行；服务端断开后剩余写入会失败，忽略即可。
	go func() {
		chunk := []byte(strings.Repeat("a", 64*1024))
		for written := 0; written <= maxRequestFrameSize; written += len(chunk) {
			if _, err := conn.Write(chunk); err != nil {
				return
			}
		}
	}()

	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set read deadline error: %v", err)
	}
	_, err = bufio.NewReader(conn).ReadBytes('\n')
	if err == nil {
		t.Fatal("超限请求帧没有导致连接断开")
	}
	if isTimeoutError(err) {
		t.Fatal("服务端在超限后仍保持连接，直到读超时")
	}

	deadline := time.Now().Add(2 * time.Second)
	for server.HasClients() {
		if time.Now().After(deadline) {
			t.Fatal("超限连接未从服务端摘除")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
