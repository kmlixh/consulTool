package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/kmlixh/consulTool/errors"
)

// RetryWithTimeout 重试函数，带超时控制
func RetryWithTimeout(ctx context.Context, operation func() error) error {
	var lastErr error
	for attempt := 0; attempt < defaultRetryAttempts; attempt++ {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("%w: %v", ctx.Err(), lastErr)
			}
			return ctx.Err()
		default:
			if err := operation(); err == nil {
				return nil
			} else {
				lastErr = err
				// 如果是已知的错误类型，直接返回
				if errors.IsConsulError(err) {
					return err
				}
				if attempt < defaultRetryAttempts-1 {
					time.Sleep(defaultRetryDelay)
				}
			}
		}
	}
	return lastErr
}
