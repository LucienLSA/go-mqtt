package middleware

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type simpleRateLimiter struct {
	tokens chan struct{}
}

func newSimpleRateLimiter(rps, burst int) *simpleRateLimiter {
	if rps <= 0 || burst <= 0 {
		return &simpleRateLimiter{tokens: nil}
	}

	if burst < rps {
		burst = rps
	}

	l := &simpleRateLimiter{tokens: make(chan struct{}, burst)}
	for i := 0; i < burst; i++ {
		l.tokens <- struct{}{}
	}

	interval := time.Second / time.Duration(rps)
	if interval <= 0 {
		interval = time.Millisecond
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			select {
			case l.tokens <- struct{}{}:
			default:
			}
		}
	}()

	return l
}

func (l *simpleRateLimiter) Allow() bool {
	if l == nil || l.tokens == nil {
		return true
	}
	select {
	case <-l.tokens:
		return true
	default:
		return false
	}
}

type circuitBreaker struct {
	mu               sync.Mutex
	failureCount     int
	openUntil        time.Time
	failureThreshold int
	openDuration     time.Duration
}

func newCircuitBreaker(failureThreshold int, openDuration time.Duration) *circuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 10
	}
	if openDuration <= 0 {
		openDuration = 15 * time.Second
	}
	return &circuitBreaker{
		failureThreshold: failureThreshold,
		openDuration:     openDuration,
	}
}

func (b *circuitBreaker) Allow() bool {
	if b == nil {
		return true
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.openUntil.IsZero() && time.Now().Before(b.openUntil) {
		return false
	}
	return true
}

func (b *circuitBreaker) RecordSuccess() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.failureCount = 0
	b.openUntil = time.Time{}
	b.mu.Unlock()
}

func (b *circuitBreaker) RecordFailure() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failureCount++
	if b.failureCount >= b.failureThreshold {
		b.openUntil = time.Now().Add(b.openDuration)
		b.failureCount = 0
	}
}

var (
	controlGuardOnce    sync.Once
	controlRateLimiter  *simpleRateLimiter
	controlCircuitBreak *circuitBreaker
)

func ControlProtection() gin.HandlerFunc {
	controlGuardOnce.Do(func() {
		rps := envInt("CTRL_RATE_LIMIT_RPS", 20)
		burst := envInt("CTRL_RATE_LIMIT_BURST", 40)
		threshold := envInt("CTRL_BREAKER_FAILURE_THRESHOLD", 10)
		openSec := envInt("CTRL_BREAKER_OPEN_SEC", 15)

		controlRateLimiter = newSimpleRateLimiter(rps, burst)
		controlCircuitBreak = newCircuitBreaker(threshold, time.Duration(openSec)*time.Second)
	})

	return func(c *gin.Context) {
		if !controlRateLimiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "请求过于频繁，请稍后重试",
				"data":    gin.H{},
			})
			return
		}

		if !controlCircuitBreak.Allow() {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"code":    503,
				"message": "控制通道暂时熔断，请稍后重试",
				"data":    gin.H{},
			})
			return
		}

		c.Next()

		status := c.Writer.Status()
		if status >= http.StatusInternalServerError {
			controlCircuitBreak.RecordFailure()
			return
		}

		if status < http.StatusTooManyRequests {
			controlCircuitBreak.RecordSuccess()
		}
	}
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
