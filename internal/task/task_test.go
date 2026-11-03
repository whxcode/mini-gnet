package task

import (
	"context"
	"errors"
	"testing"
)

// TestCancellationIsIndependent 验证停止一项不会取消其他任务，完成后等待者能读取最终错误。
func TestCancellationIsIndependent(t *testing.T) {
	first := New(context.Background(), "first")
	second := New(context.Background(), "second")
	defer second.Cancel()

	first.Cancel()

	if !errors.Is(first.Context.Err(), context.Canceled) || second.Context.Err() != nil {
		t.Fatal("任务取消信号没有隔离")
	}

	first.Complete(first.Context.Err())

	for i := 0; i < 2; i++ {
		<-first.Done

		if !errors.Is(first.Err, context.Canceled) {
			t.Fatal("完成结果丢失")
		}
	}
}
