package mrpc

import (
	"log"
	"net"
	"sync"
	"time"
)

// HealthRequest 是心跳检测的请求参数（体）。
// 可根据需要扩展字段。
type HealthRequest struct{}

// HealthReply 是心跳检测的响应参数（体）。
// Ok 为 true 表示服务健康。
type HealthReply struct {
	Ok bool
}

const (
	defaultHealthCheckInterval = 15 * time.Second
	defaultHealthCheckTimeout  = 3 * time.Second
)

// HealthChecker 提供客户端到服务端的周期性健康检查能力。
//
// 通过注册 MRPC 方法 "Health.Check"，客户端可以周期性调用此方法
// 来检测服务端是否可达。
type HealthChecker struct {
	mu       sync.Mutex
	interval time.Duration
	timeout  time.Duration
	lis      net.Listener
	done     chan struct{}
}

// NewHealthChecker 创建一个健康检查器。
//
// 参数：
//   - interval: 健康检查间隔（必须 >0，建议 5s~60s）
//   - timeout:  每次检查的超时时间（必须 >0 且 ≤ interval）
func NewHealthChecker(interval, timeout time.Duration) *HealthChecker {
	if interval <= 0 {
		interval = defaultHealthCheckInterval
	}
	if timeout <= 0 || timeout > interval {
		timeout = defaultHealthCheckTimeout
	}
	return &HealthChecker{
		interval: interval,
		timeout:  timeout,
		done:     make(chan struct{}),
	}
}

// RegisterHealth 在服务端注册心跳检测方法。
//
// 用法：
//
//	server.Register("Health", mrpc.RegisterHealth())
//
// 这会注册一个 Health.Check 方法，客户端可周期性调用以检测连通性。
//
// 返回值是指向内部实现的指针，需作为 Register 的 target 参数。
func RegisterHealth() *healthService {
	return &healthService{}
}

type healthService struct{}

// Check 是心跳检测的 RPC 方法。
//
// 签名满足 func(req HealthRequest, reply *HealthReply) error。
// 始终返回 reply.Ok = true 表示服务正常。
func (h *healthService) Check(req HealthRequest, reply *HealthReply) error {
	reply.Ok = true
	return nil
}

// Start 在后台启动心跳循环，定期向指定客户端发起健康检查调用。
//
// 当连续 maxFailures 次检查失败时，调用 onFailure 回调。
//
// 参数：
//   - client:     已连接的 RPC 客户端
//   - maxFailures: 连续失败多少次后触发回调（建议 ≥1）
//   - onFailure:   失败回调，传入最后一次错误
func (hc *HealthChecker) Start(client *Client, maxFailures int, onFailure func(error)) {
	if maxFailures <= 0 {
		maxFailures = 3
	}
	go func() {
		ticker := time.NewTicker(hc.interval)
		defer ticker.Stop()
		failures := 0

		for {
			select {
			case <-hc.done:
				return
			case <-ticker.C:
				var reply HealthReply
				err := client.Call("Health.Check", &HealthRequest{}, &reply)
				if err != nil || !reply.Ok {
					failures++
					if failures >= maxFailures {
						onFailure(err)
						failures = 0
					}
				} else {
					failures = 0
				}
				log.Printf("mrpc: health check result: ok=%v, err=%v", reply.Ok, err)
			}
		}
	}()
}

// Stop 停止心跳循环。
func (hc *HealthChecker) Stop() {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	select {
	case <-hc.done:
	default:
		close(hc.done)
	}
}
