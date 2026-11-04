package external

import (
	"bytes"
	"context"
)

// limitedOutput 收集一个输出通道；os/exec 为 stdout、stderr 分别使用独立写入协程。
type limitedOutput struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
	cancel   context.CancelFunc
}

// Write 只保留容量内的数据，超限时取消进程并继续消耗已经到达管道的数据。
func (output *limitedOutput) Write(data []byte) (int, error) {
	remaining := output.limit - output.buffer.Len()
	if len(data) > remaining {
		output.buffer.Write(data[:remaining])
		output.exceeded = true
		output.cancel()

		return len(data), nil
	}

	return output.buffer.Write(data)
}
